package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vibesort/internal/inventory"
)

func TestDecodeValidPlan(t *testing.T) {
	p, err := Decode(strings.NewReader(`{"schemaVersion":1,"file":"x.go","order":[{"id":"func:A"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.SchemaVersion != inventory.PlanSchemaVersion || p.File != "x.go" || len(p.Order) != 1 || p.Order[0].ID != "func:A" {
		t.Fatalf("plan = %#v", p)
	}
}

func TestLoadRejectsUnreadablePlanFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "read plan "+path) {
		t.Fatalf("err = %v, want read plan error", err)
	}
}

func TestDecodeRejectsInvalidPlans(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{
			name: "malformed JSON",
			json: `{`,
			want: "decode plan",
		},
		{
			name: "trailing JSON",
			json: `{"schemaVersion":1,"file":"x.go","order":[]} {}`,
			want: "trailing data",
		},
		{
			name: "missing schema version",
			json: `{"file":"x.go","order":[]}`,
			want: "missing schemaVersion",
		},
		{
			name: "null schema version",
			json: `{"schemaVersion":null,"file":"x.go","order":[]}`,
			want: "missing schemaVersion",
		},
		{
			name: "unsupported schema version",
			json: `{"schemaVersion":2,"file":"x.go","order":[]}`,
			want: "unsupported schemaVersion 2",
		},
		{
			name: "wrong schema version type",
			json: `{"schemaVersion":"1","file":"x.go","order":[]}`,
			want: "decode plan",
		},
		{
			name: "missing file",
			json: `{"schemaVersion":1,"order":[]}`,
			want: "missing file",
		},
		{
			name: "null file",
			json: `{"schemaVersion":1,"file":null,"order":[]}`,
			want: "missing file",
		},
		{
			name: "empty file",
			json: `{"schemaVersion":1,"file":"","order":[]}`,
			want: "file must not be empty",
		},
		{
			name: "whitespace-only file",
			json: `{"schemaVersion":1,"file":"   ","order":[]}`,
			want: "file must not be empty",
		},
		{
			name: "file with surrounding whitespace",
			json: `{"schemaVersion":1,"file":"x.go ","order":[]}`,
			want: "file must not have surrounding whitespace",
		},
		{
			name: "missing order",
			json: `{"schemaVersion":1,"file":"x.go"}`,
			want: "missing order",
		},
		{
			name: "null order",
			json: `{"schemaVersion":1,"file":"x.go","order":null}`,
			want: "missing order",
		},
		{
			name: "wrong order type",
			json: `{"schemaVersion":1,"file":"x.go","order":{}}`,
			want: "decode plan",
		},
		{
			name: "unknown top-level field",
			json: `{"schemaVersion":1,"file":"x.go","order":[],"language":"go"}`,
			want: `unknown field "language"`,
		},
		{
			name: "multiple unknown top-level fields",
			json: `{"schemaVersion":1,"file":"x.go","order":[],"z":0,"a":0}`,
			want: `unknown field "a"`,
		},
		{
			name: "case-mismatched top-level fields",
			json: `{"SchemaVersion":1,"File":"x.go","Order":[{"id":"func:A"}]}`,
			want: `unknown field`,
		},
		{
			name: "duplicate top-level field",
			json: `{"schemaVersion":1,"file":"reviewed.go","file":"actual.go","order":[]}`,
			want: `duplicate field "file"`,
		},
		{
			name: "duplicate schema version field",
			json: `{"schemaVersion":1,"schemaVersion":2,"file":"x.go","order":[]}`,
			want: `duplicate field "schemaVersion"`,
		},
		{
			name: "unknown order item field",
			json: `{"schemaVersion":1,"file":"x.go","order":[{"id":"func:A","name":"A"}]}`,
			want: `unknown field "name"`,
		},
		{
			name: "multiple unknown order item fields",
			json: `{"schemaVersion":1,"file":"x.go","order":[{"id":"func:A","z":0,"a":0}]}`,
			want: `unknown field "a"`,
		},
		{
			name: "case-mismatched order item field",
			json: `{"schemaVersion":1,"file":"x.go","order":[{"ID":"func:A"}]}`,
			want: `unknown field "ID"`,
		},
		{
			name: "duplicate order item field",
			json: `{"schemaVersion":1,"file":"x.go","order":[{"id":"func:A","id":"func:B"}]}`,
			want: `duplicate field "id"`,
		},
		{
			name: "missing order item id",
			json: `{"schemaVersion":1,"file":"x.go","order":[{}]}`,
			want: "missing order id at index 0",
		},
		{
			name: "null order item id",
			json: `{"schemaVersion":1,"file":"x.go","order":[{"id":null}]}`,
			want: "missing order id at index 0",
		},
		{
			name: "empty order item id",
			json: `{"schemaVersion":1,"file":"x.go","order":[{"id":""}]}`,
			want: "order id at index 0 must not be empty",
		},
		{
			name: "order item id with surrounding whitespace",
			json: `{"schemaVersion":1,"file":"x.go","order":[{"id":" func:A "}]}`,
			want: "order id at index 0 must not have surrounding whitespace",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(tc.json))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateAcceptsValidPlans(t *testing.T) {
	path := writeGoFile(t, "valid.go", mixedSource())

	cases := []struct {
		name  string
		order []string
	}{
		{
			name:  "identity order",
			order: []string{"var:0", "func:A", "func:B", "init:0"},
		},
		{
			name:  "movable reorder",
			order: []string{"func:B", "var:0", "func:A", "init:0"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Validate(newPlan(path, tc.order...))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Valid || result.File != path || result.EntityCount != 4 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestValidateAcceptsEmptyPackageWithEmptyOrder(t *testing.T) {
	path := writeGoFile(t, "empty.go", "package p\n")

	result, err := Validate(newPlan(path))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.EntityCount != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestValidateRejectsUnsupportedTargetFiles(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{
			name:    "non-Go target",
			path:    "x.txt",
			content: "not go\n",
			want:    "not a Go source file",
		},
		{
			name:    "parse error",
			path:    "parse_error.go",
			content: "package p\n\nfunc A( {}\n",
			want:    "parse Go file",
		},
		{
			name:    "non-gofmt target",
			path:    "not_gofmt.go",
			content: "package p\n\nfunc A(){}",
			want:    "source is not gofmt-clean",
		},
		{
			name:    "generated target",
			path:    "generated.go",
			content: "// Code generated by test. DO NOT EDIT.\npackage p\n\nfunc A() {}\n",
			want:    "generated",
		},
		{
			name:    "cgo target",
			path:    "cgo.go",
			content: "package p\n\nimport \"C\"\n\nfunc A() {}\n",
			want:    `import "C"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeGoFile(t, tc.path, tc.content)
			_, err := Validate(newPlan(path))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsInvalidOrders(t *testing.T) {
	path := writeGoFile(t, "invalid_order.go", mixedSource())

	cases := []struct {
		name  string
		order []string
		want  string
	}{
		{
			name:  "duplicate entity ID",
			order: []string{"var:0", "func:A", "func:A", "init:0"},
			want:  `duplicate entity id "func:A"`,
		},
		{
			name:  "unknown entity ID",
			order: []string{"var:0", "func:A", "func:C", "init:0"},
			want:  `unknown entity id "func:C"`,
		},
		{
			name:  "missing entity ID",
			order: []string{"var:0", "func:A", "init:0"},
			want:  "order length 3 does not match entity count 4",
		},
		{
			name:  "preamble sentinel is unknown",
			order: []string{"preamble", "func:A", "func:B", "init:0"},
			want:  `unknown entity id "preamble"`,
		},
		{
			name:  "postamble sentinel is unknown",
			order: []string{"postamble", "func:A", "func:B", "init:0"},
			want:  `unknown entity id "postamble"`,
		},
		{
			name:  "pinned relative order changed",
			order: []string{"init:0", "func:A", "func:B", "var:0"},
			want:  "pinned entity relative order changed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Validate(newPlan(path, tc.order...))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsStalePlan(t *testing.T) {
	path := writeGoFile(t, "stale.go", "package p\n\nfunc A() {}\n\nfunc B() {}\n")
	if _, err := Validate(newPlan(path, "func:A", "func:B")); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("package p\n\nfunc A() {}\n\nfunc C() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Validate(newPlan(path, "func:A", "func:B"))
	if err == nil || !strings.Contains(err.Error(), `unknown entity id "func:B"`) {
		t.Fatalf("err = %v, want stale unknown entity", err)
	}
}

func newPlan(path string, order ...string) *Plan {
	return &Plan{
		SchemaVersion: inventory.PlanSchemaVersion,
		File:          path,
		Order:         orderItems(order),
	}
}

func orderItems(order []string) []inventory.OrderItem {
	items := make([]inventory.OrderItem, 0, len(order))
	for _, id := range order {
		items = append(items, inventory.OrderItem{ID: id})
	}
	return items
}

func writeGoFile(t *testing.T, name, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mixedSource() string {
	return `package p

var V = 1

func A() {}

func B() {}

func init() {}
`
}

func TestResultJSONShape(t *testing.T) {
	data, err := json.Marshal(Result{Valid: true, File: "x.go", EntityCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != `{"valid":true,"file":"x.go","entityCount":2}` {
		t.Fatalf("result JSON = %s", got)
	}
}
