package inventory

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInventoryGoldenFixtures(t *testing.T) {
	cases := []struct {
		name   string
		source string
		golden string
	}{
		{
			name:   "complex mixed declarations",
			source: "complex.go",
			golden: "complex.inventory.golden.json",
		},
		{
			name:   "all top-level shapes",
			source: "all_top_level_shapes.go",
			golden: "all_top_level_shapes.inventory.golden.json",
		},
		{
			name:   "directives",
			source: "directives.go",
			golden: "directives.inventory.golden.json",
		},
		{
			name:   "blank identifiers",
			source: "blank_identifiers.go",
			golden: "blank_identifiers.inventory.golden.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := mustBuild(t, tc.source)
			golden := readGolden(t, tc.golden)

			if doc.SchemaVersion != golden.SchemaVersion {
				t.Fatalf("schema version = %d, want %d", doc.SchemaVersion, golden.SchemaVersion)
			}
			if doc.Preamble.CommentCount != golden.PreambleCommentCount {
				t.Fatalf("preamble comment count = %d, want %d", doc.Preamble.CommentCount, golden.PreambleCommentCount)
			}
			if got := readyPlanIDs(doc); !equalStrings(got, golden.ReadyPlan) {
				t.Fatalf("ready plan ids = %#v, want %#v", got, golden.ReadyPlan)
			}
			if len(doc.Entities) != len(golden.Entities) {
				t.Fatalf("entities len = %d, want %d", len(doc.Entities), len(golden.Entities))
			}

			for i, want := range golden.Entities {
				got := doc.Entities[i]
				if got.ID != want.ID || got.Kind != want.Kind || got.Movable != want.Movable || got.PinnedReason != want.PinnedReason || got.Name != want.Name || got.Receiver != want.Receiver || got.Signature != want.Signature || got.FirstDocLine != want.FirstDocLine {
					t.Fatalf("entity %d = %#v, want %#v", i, publicGoldenEntity(got), want)
				}
				if got.Index != i {
					t.Fatalf("entity %s index = %d, want %d", got.ID, got.Index, i)
				}
				if gotComments := commentTexts(got.Comments); !equalStrings(gotComments, want.Comments) {
					t.Fatalf("entity %s comments = %#v, want %#v", got.ID, gotComments, want.Comments)
				}
			}
		})
	}
}

func TestEmptyPackageInventory(t *testing.T) {
	doc := mustBuild(t, "empty_package.go")
	if len(doc.Entities) != 0 {
		t.Fatalf("entities len = %d, want 0", len(doc.Entities))
	}
	if len(doc.ReadyPlan.Order) != 0 {
		t.Fatalf("ready plan len = %d, want 0", len(doc.ReadyPlan.Order))
	}
	if doc.Preamble.CommentCount != 1 {
		t.Fatalf("preamble comment count = %d, want package doc only", doc.Preamble.CommentCount)
	}
}

func TestRejectParseErrorAndNonGo(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "parse error",
			path: fixturePath("parse_error.go"),
			want: "parse Go file",
		},
		{
			name: "non-go file",
			path: fixturePath("complex.inventory.golden.json"),
			want: "not a Go source file",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(tc.path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestCommentsAfterImportsBindToFirstEntity(t *testing.T) {
	doc := mustBuild(t, "after_imports.go")
	if len(doc.Entities) != 1 {
		t.Fatalf("entities len = %d, want 1", len(doc.Entities))
	}
	if len(doc.Entities[0].Comments) != 1 || !strings.Contains(doc.Entities[0].Comments[0].Text, "first section") {
		t.Fatalf("comment after imports should bind to first entity, got %#v", doc.Entities[0].Comments)
	}
	if doc.Preamble.CommentCount != 1 {
		t.Fatalf("preamble comment count = %d, want import trailing comment only", doc.Preamble.CommentCount)
	}
}

func TestPackageLineBlockCommentStaysInPreamble(t *testing.T) {
	src := []byte(`package foo /* trailing
continues */
func A() {}
`)
	doc := mustBuildSource(t, src)
	if doc.Preamble.CommentCount != 1 {
		t.Fatalf("preamble comment count = %d, want package-line block comment", doc.Preamble.CommentCount)
	}
	if !strings.Contains(string(doc.Preamble.segment), "continues */") {
		t.Fatalf("preamble segment = %q, want full package-line block comment", doc.Preamble.segment)
	}
	if strings.Contains(string(doc.Entities[0].segment), "continues */") {
		t.Fatalf("entity segment should not contain split package-line block comment: %q", doc.Entities[0].segment)
	}
	if len(doc.Entities[0].Comments) != 0 {
		t.Fatalf("entity comments = %#v, want none", doc.Entities[0].Comments)
	}
}

func TestReceiverNormalization(t *testing.T) {
	src := []byte(`package p

type Box[T any] struct{}

func (b *Box[T]) Get() {}
`)
	doc := mustBuildSource(t, src)
	if got := ids(doc.Entities); !equalStrings(got, []string{"type:Box", "method:Box.Get"}) {
		t.Fatalf("ids = %#v", got)
	}
	if doc.Entities[1].Receiver != "Box" {
		t.Fatalf("receiver = %q, want Box", doc.Entities[1].Receiver)
	}
}

func TestBlankIdentifierDeclarationsArePinnedAndDisambiguated(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
		ids  []string
	}{
		{
			name: "functions",
			src: []byte(`package p

func _() {}

func _() {}
`),
			ids: []string{"func:_:0", "func:_:1"},
		},
		{
			name: "types",
			src: []byte(`package p

type _ struct{}

type _ struct{}
`),
			ids: []string{"type:_:0", "type:_:1"},
		},
		{
			name: "methods",
			src: []byte(`package p

type T struct{}

func (T) _() {}

func (T) _() {}
`),
			ids: []string{"type:T", "method:T._:0", "method:T._:1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := mustBuildSource(t, tc.src)
			if got := ids(doc.Entities); !equalStrings(got, tc.ids) {
				t.Fatalf("ids = %#v, want %#v", got, tc.ids)
			}
			for _, entity := range doc.Entities {
				if entity.Name != "_" {
					continue
				}
				if entity.Movable || entity.PinnedReason != blankIdentifierPinnedReason {
					t.Fatalf("%s movable=%v pinnedReason=%q, want pinned blank identifier", entity.ID, entity.Movable, entity.PinnedReason)
				}
			}
		})
	}
}

func TestRejectGeneratedAndCgo(t *testing.T) {
	cases := []struct {
		name string
		file string
		want string
	}{
		{
			name: "generated",
			file: "generated.go",
			want: "generated",
		},
		{
			name: "cgo",
			file: "cgo.go",
			want: "import \"C\"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(fixturePath(tc.file))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestGeneratedDetectionUsesParsedComments(t *testing.T) {
	src := []byte(`/*
package oldnote
*/
// Code generated by tool. DO NOT EDIT.
package real

func Foo() {}
`)
	if _, err := BuildSource("x.go", "/tmp/x.go", src); err == nil || !strings.Contains(err.Error(), "generated") {
		t.Fatalf("generated err = %v, want generated rejection", err)
	}
}

func TestRejectIDCollisionAndNonGofmtSource(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "id collision",
			src:  "package p\n\nfunc A() {}\n\nfunc A() {}\n",
			want: "entity id collision: func:A",
		},
		{
			name: "same-line top-level declarations",
			src:  "package p\n\ntype A int; type B int\n",
			want: "source is not gofmt-clean",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildSource("x.go", "/tmp/x.go", []byte(tc.src))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestUnknownGoDirectivePinsEntity(t *testing.T) {
	cases := []struct {
		name   string
		src    []byte
		reason string
	}{
		{
			name: "leading unknown directive",
			src: []byte(`package p

//go:unknown
func A() {}
`),
			reason: "go directive: unknown go:unknown",
		},
		{
			name: "recognized directive wins",
			src: []byte(`package p

//go:noinline
//go:unknown
func A() {}
`),
			reason: "go directive: go:noinline",
		},
		{
			name: "leading made up directive",
			src: []byte(`package p

//go:somethingmadeup
func A() {}
`),
			reason: "go directive: unknown go:somethingmadeup",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := mustBuildSource(t, tc.src)
			assertEntity(t, doc.Entities[0], "func", false, tc.reason)
		})
	}
}

func TestGoLineDirectivePinsActiveFile(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
	}{
		{
			name: "line comment",
			src: []byte(`package p

func A() {}

//line generated.go:100
func B() {}

func C() {}
`),
		},
		{
			name: "block comment",
			src: []byte(`package p

func A() {}

/*line generated.go:200*/
func B() {}

func C() {}
`),
		},
		{
			name: "block comment in body",
			src: []byte(`package p

func A() {
	/*line generated.go:300*/
	_ = 1
}

func B() {}
`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := mustBuildSource(t, tc.src)
			for _, entity := range doc.Entities {
				if entity.Movable || entity.PinnedReason != lineDirectivePinnedReason {
					t.Fatalf("%s movable=%v pinnedReason=%q, want line directive pin", entity.ID, entity.Movable, entity.PinnedReason)
				}
			}

			reversed := make([]string, 0, len(doc.Entities))
			for i := len(doc.Entities) - 1; i >= 0; i-- {
				reversed = append(reversed, doc.Entities[i].ID)
			}
			_, err := Reassemble(doc, reversed)
			if err == nil || !strings.Contains(err.Error(), "pinned entity relative order changed") {
				t.Fatalf("Reassemble reversed order err = %v, want pinned order error", err)
			}
		})
	}
}

func TestLineLikeOrdinaryCommentsDoNotPinEntity(t *testing.T) {
	src := []byte(`package p

// lineage note
func A() {}

//line no-colon
func B() {}

func C() {} //line generated.go:100
`)
	doc := mustBuildSource(t, src)
	for _, entity := range doc.Entities {
		if !entity.Movable || entity.PinnedReason != "" {
			t.Fatalf("%s movable=%v pinnedReason=%q, want ordinary comment", entity.ID, entity.Movable, entity.PinnedReason)
		}
	}
}

func TestLeadingExportCommentIsOrdinaryNonCgoComment(t *testing.T) {
	src := []byte(`package p

//export Foo
func Foo() {}
`)
	doc := mustBuildSource(t, src)
	assertEntity(t, doc.Entities[0], "func", true, "")
}

func TestSameLineTrailingGoDirectiveIsOrdinaryComment(t *testing.T) {
	src := []byte(`package p

func Foo() {} //go:noinline
`)
	doc := mustBuildSource(t, src)
	assertEntity(t, doc.Entities[0], "func", true, "")
}

func TestTrailingGenerateDirectiveBecomesPostamble(t *testing.T) {
	src := []byte(`package p

func A() {}

func B() {}

//go:generate echo hi
`)
	doc := mustBuildSource(t, src)
	if len(doc.Entities) != 2 {
		t.Fatalf("entities len = %d, want 2", len(doc.Entities))
	}
	assertEntity(t, doc.Entities[1], "func", true, "")
	if doc.Postamble == nil || doc.Postamble.CommentCount != 1 || !strings.Contains(string(doc.Postamble.segment), "//go:generate echo hi") {
		t.Fatalf("postamble = %#v, want trailing go:generate", doc.Postamble)
	}
}

func TestTrailingUnknownDirectiveBecomesPostamble(t *testing.T) {
	src := []byte(`package p

func A() {}

//go:unknown
`)
	doc := mustBuildSource(t, src)
	assertEntity(t, doc.Entities[0], "func", true, "")
	if doc.Postamble == nil || doc.Postamble.CommentCount != 1 || !strings.Contains(string(doc.Postamble.segment), "//go:unknown") {
		t.Fatalf("postamble = %#v, want trailing unknown directive", doc.Postamble)
	}
}

func TestUnstarredBlockDocSignature(t *testing.T) {
	src := []byte(`package p

/*
A does something.
It has details.
*/
func A() {}
`)
	doc := mustBuildSource(t, src)
	if got := doc.Entities[0].Signature; got != "func A() {}" {
		t.Fatalf("signature = %q, want declaration line", got)
	}
}

func TestDirectiveLikeCommentsInsideBodiesIgnored(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "unsupported go directive",
			body: "\t//go:frobnicate",
		},
		{
			name: "cgo export",
			body: "\t//export Foo",
		},
		{
			name: "recognized go directive",
			body: "\t//go:noinline",
		},
		{
			name: "block comment text",
			body: "\t/*\n\t\t//go:noinline\n\t*/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte("package p\n\nfunc A() {\n" + tc.body + "\n}\n")
			doc, err := BuildSource("x.go", "/tmp/x.go", src)
			if err != nil {
				t.Fatal(err)
			}
			assertEntity(t, doc.Entities[0], "func", true, "")
		})
	}
}

func TestRejectNonGofmtCleanSource(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
	}{
		{
			name: "same-line preamble declaration",
			src:  []byte("package p; func A() {}\n"),
		},
		{
			name: "missing trailing newline",
			src:  []byte("package p\n\nfunc A() {}"),
		},
		{
			name: "crlf line endings",
			src:  []byte("package p\r\n\r\nfunc A() {}\r\n"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildSource("x.go", "/tmp/x.go", tc.src)
			if err == nil || !strings.Contains(err.Error(), "source is not gofmt-clean") {
				t.Fatalf("err = %v, want gofmt-clean rejection", err)
			}
		})
	}
}

func TestRejectSegmentMissingDeclaration(t *testing.T) {
	src := []byte(`package p

func A() {} /* block
 */func B() {}
`)
	_, err := BuildSource("x.go", "/tmp/x.go", src)
	if err == nil || !strings.Contains(err.Error(), "declaration is outside its source segment") {
		t.Fatalf("err = %v, want invalid segment rejection", err)
	}
}

func TestReassembleRequiresCompletePermutation(t *testing.T) {
	doc := mustBuildSource(t, []byte("package p\n\nfunc A() {}\n\nfunc B() {}\n"))

	cases := []struct {
		name  string
		order []string
		want  string
	}{
		{
			name:  "missing",
			order: []string{"func:A"},
			want:  "order length 1 does not match entity count 2",
		},
		{
			name:  "duplicate",
			order: []string{"func:A", "func:A"},
			want:  "duplicate entity id",
		},
		{
			name:  "unknown",
			order: []string{"func:A", "func:C"},
			want:  "unknown entity id",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Reassemble(doc, tc.order)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestReassembleRejectsPinnedRelativeReorder(t *testing.T) {
	doc := mustBuildSource(t, []byte(`package p

func init() { println(1) }

func init() { println(2) }
`))

	_, err := Reassemble(doc, []string{"init:1", "init:0"})
	if err == nil || !strings.Contains(err.Error(), "pinned entity relative order changed") {
		t.Fatalf("err = %v, want pinned order rejection", err)
	}
}

func TestReassembleRejectsJSONRoundTrippedDocument(t *testing.T) {
	doc := mustBuildSource(t, []byte("package p\n\nfunc A() {}\n\nfunc B() {}\n"))
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Document
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	_, err = Reassemble(&decoded, readyPlanIDs(&decoded))
	if err == nil || !strings.Contains(err.Error(), "source segments are unavailable") {
		t.Fatalf("err = %v, want missing segment rejection", err)
	}
}

func TestReassembleKeepsPostambleLast(t *testing.T) {
	src := []byte(`package p

func A() {}

func B() {}

// trailing eof comment
`)
	doc := mustBuildSource(t, src)
	if doc.Postamble == nil || doc.Postamble.CommentCount != 1 {
		t.Fatalf("postamble = %#v, want trailing comment postamble", doc.Postamble)
	}
	if len(doc.Entities[1].Comments) != 0 {
		t.Fatalf("B comments = %#v, want trailing comment excluded from entity", doc.Entities[1].Comments)
	}

	out, err := Reassemble(doc, []string{"func:B", "func:A"})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`package p

func B() {}

func A() {}

// trailing eof comment
`)
	if !bytes.Equal(out, want) {
		t.Fatalf("permuted output mismatch\n got: %q\nwant: %q", out, want)
	}
}

func TestPermutedReassemblyGolden(t *testing.T) {
	src := []byte("package p\n\n" +
		"func B() {} // trailing -> B\n" +
		"\n" +
		"// ---- section ---- -> C\n" +
		"\n" +
		"// C doc -> C\n" +
		"func C() {}\n")

	doc := mustBuildSource(t, src)
	if got := ids(doc.Entities); !equalStrings(got, []string{"func:B", "func:C"}) {
		t.Fatalf("ids = %#v", got)
	}
	if got := doc.Entities[0].Comments; len(got) != 1 || !strings.Contains(got[0].Text, "trailing -> B") {
		t.Fatalf("B comments = %#v, want trailing only", got)
	}
	if got := doc.Entities[1].Comments; len(got) != 2 {
		t.Fatalf("C comments len = %d, want section and doc: %#v", len(got), got)
	}

	out, err := Reassemble(doc, []string{"func:C", "func:B"})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("package p\n\n" +
		"// ---- section ---- -> C\n" +
		"\n" +
		"// C doc -> C\n" +
		"func C() {}\n" +
		"\n" +
		"func B() {} // trailing -> B\n")
	if !bytes.Equal(out, want) {
		t.Fatalf("permuted output mismatch\n got: %q\nwant: %q", out, want)
	}
}

func TestSingleSpecGroupedTypePinned(t *testing.T) {
	src := []byte(`package p

type (
	Only struct{}
)
`)
	doc := mustBuildSource(t, src)
	if got := ids(doc.Entities); !equalStrings(got, []string{"type_group:0"}) {
		t.Fatalf("ids = %#v", got)
	}
	assertEntity(t, doc.Entities[0], "type_group", false, "grouped type declarations are pinned in v1")
}

type inventoryGolden struct {
	SchemaVersion        int            `json:"schemaVersion"`
	PreambleCommentCount int            `json:"preambleCommentCount"`
	ReadyPlan            []string       `json:"readyPlan"`
	Entities             []goldenEntity `json:"entities"`
}

type goldenEntity struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Movable      bool     `json:"movable"`
	PinnedReason string   `json:"pinnedReason"`
	Name         string   `json:"name"`
	Receiver     string   `json:"receiver"`
	Signature    string   `json:"signature"`
	FirstDocLine string   `json:"firstDocLine"`
	Comments     []string `json:"comments"`
}

func mustBuild(t *testing.T, name string) *Document {
	t.Helper()
	doc, err := Build(fixturePath(name))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func mustBuildSource(t *testing.T, src []byte) *Document {
	t.Helper()
	doc, err := BuildSource("x.go", "/tmp/x.go", src)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func readGolden(t *testing.T, name string) inventoryGolden {
	t.Helper()
	data, err := os.ReadFile(fixturePath(name))
	if err != nil {
		t.Fatal(err)
	}
	var golden inventoryGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

func fixturePath(name string) string {
	return filepath.Join("testdata", name)
}

func ids(entities []Entity) []string {
	out := make([]string, 0, len(entities))
	for _, entity := range entities {
		out = append(out, entity.ID)
	}
	return out
}

func readyPlanIDs(doc *Document) []string {
	out := make([]string, 0, len(doc.ReadyPlan.Order))
	for _, item := range doc.ReadyPlan.Order {
		out = append(out, item.ID)
	}
	return out
}

func commentTexts(comments []CommentGroup) []string {
	out := make([]string, 0, len(comments))
	for _, comment := range comments {
		out = append(out, comment.Text)
	}
	return out
}

func publicGoldenEntity(entity Entity) goldenEntity {
	return goldenEntity{
		ID:           entity.ID,
		Kind:         entity.Kind,
		Movable:      entity.Movable,
		PinnedReason: entity.PinnedReason,
		Name:         entity.Name,
		Receiver:     entity.Receiver,
		Signature:    entity.Signature,
		FirstDocLine: entity.FirstDocLine,
		Comments:     commentTexts(entity.Comments),
	}
}

func assertEntity(t *testing.T, entity Entity, kind string, movable bool, pinnedReason string) {
	t.Helper()
	if entity.Kind != kind || entity.Movable != movable || entity.PinnedReason != pinnedReason {
		t.Fatalf("%s = kind %q movable %v pinnedReason %q; want %q %v %q", entity.ID, entity.Kind, entity.Movable, entity.PinnedReason, kind, movable, pinnedReason)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
