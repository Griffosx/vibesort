package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const SchemaVersion = 1

const blankIdentifierPinnedReason = "blank identifier declaration has no stable name"
const lineDirectivePinnedReason = "go directive: line directive"

type Document struct {
	SchemaVersion int        `json:"schemaVersion"`
	File          string     `json:"file"`
	ResolvedFile  string     `json:"resolvedFile"`
	Preamble      Preamble   `json:"preamble"`
	Postamble     *Postamble `json:"postamble,omitempty"`
	Entities      []Entity   `json:"entities"`
	ReadyPlan     ReadyPlan  `json:"readyPlan"`

	source []byte
}

type Preamble struct {
	Kind         string `json:"kind"`
	Fixed        bool   `json:"fixed"`
	Span         Span   `json:"span"`
	CommentCount int    `json:"commentCount"`

	segment []byte
}

type Postamble struct {
	Kind         string `json:"kind"`
	Fixed        bool   `json:"fixed"`
	Span         Span   `json:"span"`
	CommentCount int    `json:"commentCount"`

	segment []byte
}

type Entity struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Index        int            `json:"index"`
	Movable      bool           `json:"movable"`
	PinnedReason string         `json:"pinnedReason,omitempty"`
	Name         string         `json:"name,omitempty"`
	Receiver     string         `json:"receiver,omitempty"`
	Signature    string         `json:"signature,omitempty"`
	FirstDocLine string         `json:"firstDocLine,omitempty"`
	Span         Span           `json:"span"`
	Comments     []CommentGroup `json:"comments,omitempty"`

	segment []byte
}

type Span struct {
	StartByte int      `json:"startByte"`
	EndByte   int      `json:"endByte"`
	Start     Position `json:"start"`
	End       Position `json:"end"`
}

type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type CommentGroup struct {
	Text string `json:"text"`
	Span Span   `json:"span"`
}

type ReadyPlan struct {
	File  string      `json:"file"`
	Order []OrderItem `json:"order"`
}

type OrderItem struct {
	ID string `json:"id"`
}

type offsets struct {
	file *token.File
	fset *token.FileSet
}

func Build(path string) (*Document, error) {
	if filepath.Ext(path) != ".go" {
		return nil, fmt.Errorf("%s is not a Go source file", path)
	}

	resolved, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	src, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return BuildSource(path, resolved, src)
}

func BuildSource(path, resolved string, src []byte) (*Document, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, resolved, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse Go file: %w", err)
	}

	if err := rejectNonGofmt(src); err != nil {
		return nil, err
	}

	if ast.IsGenerated(file) {
		return nil, errors.New("generated files are not supported")
	}

	if importsC(file) {
		return nil, errors.New("cgo files with import \"C\" are not supported")
	}

	tokFile := fset.File(file.Package)
	if tokFile == nil {
		return nil, errors.New("could not resolve token file")
	}
	off := offsets{file: tokFile, fset: fset}

	decls := postPreambleDecls(file)
	if err := rejectSameLineDecls(decls, off); err != nil {
		return nil, err
	}
	preambleEnd := len(src)
	postambleStart := len(src)
	if len(decls) > 0 {
		preambleEnd = fixedPreambleEnd(src, file, decls[0], off)
		postambleStart = fixedPostambleStart(src, decls[len(decls)-1], file.Comments, off)
	}

	doc := &Document{
		SchemaVersion: SchemaVersion,
		File:          path,
		ResolvedFile:  resolved,
		Preamble: Preamble{
			Kind:         "preamble",
			Fixed:        true,
			Span:         spanFor(0, preambleEnd, off),
			CommentCount: countCommentsInRange(file.Comments, 0, preambleEnd, off),
			segment:      append([]byte(nil), src[:preambleEnd]...),
		},
		source: append([]byte(nil), src...),
	}
	if postambleStart < len(src) {
		doc.Postamble = &Postamble{
			Kind:         "postamble",
			Fixed:        true,
			Span:         spanFor(postambleStart, len(src), off),
			CommentCount: countCommentsInRange(file.Comments, postambleStart, len(src), off),
			segment:      append([]byte(nil), src[postambleStart:]...),
		}
	}

	entities, err := buildEntities(src, file, decls, preambleEnd, postambleStart, off)
	if err != nil {
		return nil, err
	}
	if hasLineDirectiveBefore(file.Comments, postambleStart, off) {
		pinEntitiesForLineDirective(entities)
	}
	doc.Entities = entities
	for _, entity := range entities {
		doc.ReadyPlan.Order = append(doc.ReadyPlan.Order, OrderItem{ID: entity.ID})
	}
	doc.ReadyPlan.File = path

	if err := doc.verifyIdentity(); err != nil {
		return nil, err
	}

	return doc, nil
}

func Reassemble(doc *Document, order []string) ([]byte, error) {
	if len(order) != len(doc.Entities) {
		return nil, fmt.Errorf("order length %d does not match entity count %d", len(order), len(doc.Entities))
	}
	if err := validateReassembleSegments(doc); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.Write(doc.Preamble.segment)

	byID := make(map[string]Entity, len(doc.Entities))
	for _, entity := range doc.Entities {
		byID[entity.ID] = entity
	}
	seen := make(map[string]struct{}, len(order))
	var previousSegment []byte
	pinnedOrder := []string{}

	for _, id := range order {
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate entity id %q", id)
		}
		seen[id] = struct{}{}

		entity, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown entity id %q", id)
		}
		if !entity.Movable {
			pinnedOrder = append(pinnedOrder, entity.ID)
		}
		writeEntityBoundary(&out, previousSegment, entity.segment)
		out.Write(entity.segment)
		previousSegment = entity.segment
	}

	for _, entity := range doc.Entities {
		if _, ok := seen[entity.ID]; !ok {
			return nil, fmt.Errorf("missing entity id %q", entity.ID)
		}
	}

	if !sameStrings(pinnedOrder, originalPinnedOrder(doc.Entities)) {
		return nil, errors.New("pinned entity relative order changed")
	}
	if doc.Postamble != nil {
		out.Write(doc.Postamble.segment)
	}

	return out.Bytes(), nil
}

func rejectNonGofmt(src []byte) error {
	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("check gofmt: %w", err)
	}
	if !bytes.Equal(formatted, src) {
		return errors.New("source is not gofmt-clean; run gofmt before vibesort")
	}
	return nil
}

func validateReassembleSegments(doc *Document) error {
	if len(doc.Preamble.segment) == 0 {
		return errors.New("document source segments are unavailable; rebuild inventory from source before reassemble")
	}
	for _, entity := range doc.Entities {
		if len(entity.segment) == 0 {
			return fmt.Errorf("entity %q has no source segment", entity.ID)
		}
	}
	if doc.Postamble != nil && len(doc.Postamble.segment) == 0 && doc.Postamble.Span.StartByte != doc.Postamble.Span.EndByte {
		return errors.New("postamble source segment is unavailable; rebuild inventory from source before reassemble")
	}
	return nil
}

func originalPinnedOrder(entities []Entity) []string {
	out := []string{}
	for _, entity := range entities {
		if !entity.Movable {
			out = append(out, entity.ID)
		}
	}
	return out
}

func sameStrings(a, b []string) bool {
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

func writeEntityBoundary(out *bytes.Buffer, previous, next []byte) {
	if len(previous) == 0 || len(next) == 0 || endsLineBreak(previous) {
		return
	}
	out.Write(lineBreakFor(previous, next))
}

func endsLineBreak(segment []byte) bool {
	return len(segment) > 0 && segment[len(segment)-1] == '\n'
}

func lineBreakFor(a, b []byte) []byte {
	if bytes.Contains(a, []byte("\r\n")) || bytes.Contains(b, []byte("\r\n")) {
		return []byte("\r\n")
	}
	return []byte("\n")
}

func (d *Document) verifyIdentity() error {
	order := make([]string, 0, len(d.Entities))
	for _, entity := range d.Entities {
		order = append(order, entity.ID)
	}

	out, err := Reassemble(d, order)
	if err != nil {
		return err
	}
	if !bytes.Equal(out, d.source) {
		return errors.New("inventory identity round-trip mismatch")
	}
	return nil
}

func postPreambleDecls(file *ast.File) []ast.Decl {
	decls := make([]ast.Decl, 0, len(file.Decls))
	seenNonImport := false
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT && !seenNonImport {
			continue
		}
		seenNonImport = true
		decls = append(decls, decl)
	}
	return decls
}

func rejectSameLineDecls(decls []ast.Decl, off offsets) error {
	if len(decls) < 2 {
		return nil
	}
	previousLine := off.fset.PositionFor(decls[0].Pos(), false).Line
	for _, decl := range decls[1:] {
		line := off.fset.PositionFor(decl.Pos(), false).Line
		if line == previousLine {
			return fmt.Errorf("multiple top-level declarations on line %d are not supported", line)
		}
		previousLine = line
	}
	return nil
}

func fixedPreambleEnd(src []byte, file *ast.File, firstPost ast.Decl, off offsets) int {
	var lastImport ast.Decl
	for _, decl := range file.Decls {
		if decl == firstPost {
			break
		}
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			lastImport = decl
		}
	}

	if lastImport != nil {
		return declOwnedLineEnd(src, lastImport, file.Comments, off)
	}
	return packageOwnedLineEnd(src, file, off)
}

func fixedPostambleStart(src []byte, lastPost ast.Decl, comments []*ast.CommentGroup, off offsets) int {
	return declOwnedLineEnd(src, lastPost, comments, off)
}

func packageOwnedLineEnd(src []byte, file *ast.File, off offsets) int {
	packageEnd := off.file.Offset(file.Name.End())
	packageEndLine := off.fset.PositionFor(file.Name.End(), false).Line
	end := lineEndAfter(src, packageEnd)

	for _, group := range file.Comments {
		groupStart := off.file.Offset(group.Pos())
		if groupStart < packageEnd {
			continue
		}
		groupStartLine := off.fset.PositionFor(group.Pos(), false).Line
		if groupStartLine != packageEndLine {
			continue
		}
		groupEnd := off.file.Offset(group.End())
		if lineEnd := lineEndAfter(src, groupEnd); lineEnd > end {
			end = lineEnd
		}
	}

	return end
}

func buildEntities(src []byte, file *ast.File, decls []ast.Decl, preambleEnd, postambleStart int, off offsets) ([]Entity, error) {
	entities := make([]Entity, 0, len(decls))
	seenIDs := map[string]struct{}{}
	ordinals := map[string]int{}

	start := preambleEnd
	for i, decl := range decls {
		end := postambleStart
		if i < len(decls)-1 {
			end = declOwnedLineEnd(src, decl, file.Comments, off)
		}
		if start < 0 || end < start || end > len(src) {
			return nil, fmt.Errorf("invalid segment bounds for entity %d", i)
		}
		declStart := off.file.Offset(decl.Pos())
		if start == end || declStart < start || declStart >= end {
			return nil, fmt.Errorf("invalid segment for entity %d: declaration is outside its source segment", i)
		}

		entity, err := classifyEntity(decl, i, ordinals, src[start:end], start, end, file.Comments, off)
		if err != nil {
			return nil, err
		}
		if _, exists := seenIDs[entity.ID]; exists {
			return nil, fmt.Errorf("entity id collision: %s", entity.ID)
		}
		seenIDs[entity.ID] = struct{}{}

		entities = append(entities, entity)
		start = end
	}

	return entities, nil
}

func pinEntitiesForLineDirective(entities []Entity) {
	for i := range entities {
		if entities[i].Movable {
			entities[i].Movable = false
			entities[i].PinnedReason = lineDirectivePinnedReason
		}
	}
}

func classifyEntity(decl ast.Decl, index int, ordinals map[string]int, segment []byte, start, end int, comments []*ast.CommentGroup, off offsets) (Entity, error) {
	entity := Entity{
		Index:     index,
		Span:      spanFor(start, end, off),
		Comments:  commentGroupsInRange(comments, start, end, off),
		Signature: declSignatureLine(segment, start, decl, off),
		segment:   append([]byte(nil), segment...),
	}

	switch d := decl.(type) {
	case *ast.FuncDecl:
		entity.Name = d.Name.Name
		entity.FirstDocLine = firstDocLine(d.Doc)
		if d.Recv != nil {
			entity.Kind = "method"
			entity.Receiver = normalizeReceiver(d.Recv)
			if d.Name.Name == "_" {
				entity.ID = ordinalID(fmt.Sprintf("method:%s._", entity.Receiver), ordinals)
				entity.PinnedReason = blankIdentifierPinnedReason
			} else {
				entity.ID = fmt.Sprintf("method:%s.%s", entity.Receiver, entity.Name)
				entity.Movable = true
			}
		} else if d.Name.Name == "init" {
			entity.Kind = "init"
			entity.ID = ordinalID("init", ordinals)
			entity.PinnedReason = "init order is significant"
		} else if d.Name.Name == "_" {
			entity.Kind = "func"
			entity.ID = ordinalID("func:_", ordinals)
			entity.PinnedReason = blankIdentifierPinnedReason
		} else {
			entity.Kind = "func"
			entity.ID = "func:" + d.Name.Name
			entity.Movable = true
		}
	case *ast.GenDecl:
		entity.FirstDocLine = firstDocLine(d.Doc)
		switch d.Tok {
		case token.VAR:
			entity.Kind = "var"
			entity.ID = ordinalID("var", ordinals)
			entity.PinnedReason = "package var order can be significant"
			entity.Name = genDeclName(d)
		case token.CONST:
			entity.Kind = "const"
			entity.ID = ordinalID("const", ordinals)
			entity.PinnedReason = "package const order can be significant"
			entity.Name = genDeclName(d)
		case token.TYPE:
			if d.Lparen.IsValid() || len(d.Specs) != 1 {
				entity.Kind = "type_group"
				entity.ID = ordinalID("type_group", ordinals)
				entity.PinnedReason = "grouped type declarations are pinned in v1"
				entity.Name = genDeclName(d)
				break
			}
			spec, ok := d.Specs[0].(*ast.TypeSpec)
			if !ok {
				return Entity{}, errors.New("unexpected non-type spec in type declaration")
			}
			entity.Kind = "type"
			entity.Name = spec.Name.Name
			if spec.Name.Name == "_" {
				entity.ID = ordinalID("type:_", ordinals)
				entity.PinnedReason = blankIdentifierPinnedReason
			} else {
				entity.ID = "type:" + spec.Name.Name
				entity.Movable = true
			}
			if entity.FirstDocLine == "" {
				entity.FirstDocLine = firstDocLine(spec.Doc)
			}
		default:
			return Entity{}, fmt.Errorf("unsupported declaration token %s", d.Tok)
		}
	default:
		return Entity{}, fmt.Errorf("unsupported declaration type %T", decl)
	}

	directive := scanDirectives(directiveCommentsForDecl(decl, start, end, comments, off), off)
	if directive != "" {
		entity.Movable = false
		entity.PinnedReason = "go directive: " + directive
	}

	return entity, nil
}

func ordinalID(kind string, ordinals map[string]int) string {
	n := ordinals[kind]
	ordinals[kind] = n + 1
	return kind + ":" + strconv.Itoa(n)
}

func declOwnedLineEnd(src []byte, decl ast.Decl, comments []*ast.CommentGroup, off offsets) int {
	declEnd := off.file.Offset(decl.End())
	declEndLine := off.fset.PositionFor(decl.End(), false).Line
	end := lineEndAfter(src, declEnd)

	for _, group := range comments {
		groupStart := off.file.Offset(group.Pos())
		if groupStart < declEnd {
			continue
		}
		groupStartLine := off.fset.PositionFor(group.Pos(), false).Line
		if groupStartLine != declEndLine {
			continue
		}
		groupEnd := off.file.Offset(group.End())
		if lineEnd := lineEndAfter(src, groupEnd); lineEnd > end {
			end = lineEnd
		}
	}

	return end
}

func lineEndAfter(src []byte, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset >= len(src) {
		return len(src)
	}
	if i := bytes.IndexByte(src[offset:], '\n'); i >= 0 {
		return offset + i + 1
	}
	return len(src)
}

func spanFor(start, end int, off offsets) Span {
	return Span{
		StartByte: start,
		EndByte:   end,
		Start:     positionForOffset(start, off),
		End:       positionForOffset(end, off),
	}
}

func positionForOffset(offset int, off offsets) Position {
	pos := off.file.Pos(offset)
	p := off.fset.PositionFor(pos, false)
	return Position{Line: p.Line, Column: p.Column}
}

func countCommentsInRange(groups []*ast.CommentGroup, start, end int, off offsets) int {
	count := 0
	for _, group := range groups {
		groupStart := off.file.Offset(group.Pos())
		groupEnd := off.file.Offset(group.End())
		if groupStart >= start && groupEnd <= end {
			count++
		}
	}
	return count
}

func commentGroupsInRange(groups []*ast.CommentGroup, start, end int, off offsets) []CommentGroup {
	out := []CommentGroup{}
	for _, group := range groups {
		groupStart := off.file.Offset(group.Pos())
		groupEnd := off.file.Offset(group.End())
		if groupStart < start || groupEnd > end {
			continue
		}
		out = append(out, CommentGroup{
			Text: rawCommentText(group),
			Span: spanFor(groupStart, groupEnd, off),
		})
	}
	return out
}

func rawCommentText(group *ast.CommentGroup) string {
	parts := make([]string, 0, len(group.List))
	for _, comment := range group.List {
		parts = append(parts, comment.Text)
	}
	return strings.Join(parts, "\n")
}

func firstDocLine(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	for _, line := range strings.Split(group.Text(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func declSignatureLine(segment []byte, segmentStart int, decl ast.Decl, off offsets) string {
	declStart := off.file.Offset(decl.Pos()) - segmentStart
	if declStart < 0 || declStart >= len(segment) {
		return ""
	}
	return strings.TrimSpace(string(segment[declStart:lineEndAfter(segment, declStart)]))
}

func directiveCommentsForDecl(decl ast.Decl, start, end int, groups []*ast.CommentGroup, off offsets) []*ast.Comment {
	declStart := off.file.Offset(decl.Pos())
	declEnd := off.file.Offset(decl.End())
	comments := []*ast.Comment{}
	seen := map[*ast.Comment]struct{}{}

	addGroup := func(group *ast.CommentGroup) {
		if group == nil {
			return
		}
		groupStart := off.file.Offset(group.Pos())
		groupEnd := off.file.Offset(group.End())
		if groupStart < start || groupEnd > end {
			return
		}
		for _, comment := range group.List {
			if _, ok := seen[comment]; ok {
				continue
			}
			seen[comment] = struct{}{}
			comments = append(comments, comment)
		}
	}

	for _, group := range groups {
		groupStart := off.file.Offset(group.Pos())
		groupEnd := off.file.Offset(group.End())
		if groupStart < start || groupEnd > end {
			continue
		}
		if groupEnd <= declStart {
			addGroup(group)
			continue
		}
		if groupStart >= declEnd && off.fset.PositionFor(group.Pos(), false).Column == 1 {
			addGroup(group)
		}
	}

	if gen, ok := decl.(*ast.GenDecl); ok {
		for _, spec := range gen.Specs {
			switch s := spec.(type) {
			case *ast.ValueSpec:
				addGroup(s.Doc)
			case *ast.TypeSpec:
				addGroup(s.Doc)
			}
		}
	}

	return comments
}

func hasLineDirectiveBefore(groups []*ast.CommentGroup, end int, off offsets) bool {
	for _, group := range groups {
		for _, comment := range group.List {
			if off.file.Offset(comment.Slash) >= end {
				continue
			}
			if isLineDirective(comment, off) {
				return true
			}
		}
	}
	return false
}

func genDeclName(decl *ast.GenDecl) string {
	names := []string{}
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			for _, name := range s.Names {
				names = append(names, name.Name)
			}
		case *ast.TypeSpec:
			names = append(names, s.Name.Name)
		}
	}
	return strings.Join(names, ",")
}

func normalizeReceiver(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return normalizeReceiverExpr(recv.List[0].Type)
}

func normalizeReceiverExpr(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return normalizeReceiverExpr(t.X)
	case *ast.ParenExpr:
		return normalizeReceiverExpr(t.X)
	case *ast.IndexExpr:
		return normalizeReceiverExpr(t.X)
	case *ast.IndexListExpr:
		return normalizeReceiverExpr(t.X)
	case *ast.Ident:
		return t.Name
	default:
		var buf bytes.Buffer
		_ = printer.Fprint(&buf, token.NewFileSet(), expr)
		text := strings.TrimPrefix(buf.String(), "*")
		if i := strings.IndexByte(text, '['); i >= 0 {
			text = text[:i]
		}
		return text
	}
}

func scanDirectives(comments []*ast.Comment, off offsets) string {
	recognized := map[string]struct{}{
		"go:debug":          {},
		"go:embed":          {},
		"go:generate":       {},
		"go:linkname":       {},
		"go:noescape":       {},
		"go:noinline":       {},
		"go:norace":         {},
		"go:nosplit":        {},
		"go:notinheap":      {},
		"go:uintptrescapes": {},
		"go:wasmexport":     {},
		"go:wasmimport":     {},
	}

	var firstDirective string
	for _, comment := range comments {
		line := comment.Text
		if isLineDirective(comment, off) {
			if firstDirective == "" {
				firstDirective = "line directive"
			}
			continue
		}
		if !strings.HasPrefix(line, "//go:") {
			continue
		}
		name := strings.TrimPrefix(line, "//")
		if fields := strings.Fields(name); len(fields) > 0 {
			name = fields[0]
		}
		if name == "go:generate" && off.fset.PositionFor(comment.Slash, false).Column != 1 {
			continue
		}
		if firstDirective == "" {
			if _, ok := recognized[name]; ok {
				firstDirective = name
			} else {
				firstDirective = "unknown " + name
			}
		}
	}

	return firstDirective
}

func isLineDirective(comment *ast.Comment, off offsets) bool {
	text := comment.Text
	switch {
	case strings.HasPrefix(text, "//line "):
		return off.fset.PositionFor(comment.Slash, false).Column == 1 && strings.Contains(text, ":")
	case strings.HasPrefix(text, "/*line "):
		return strings.HasSuffix(text, "*/") && strings.Contains(text, ":")
	default:
		return false
	}
}

func importsC(file *ast.File) bool {
	for _, imp := range file.Imports {
		if imp.Path != nil && imp.Path.Value == `"C"` {
			return true
		}
	}
	return false
}
