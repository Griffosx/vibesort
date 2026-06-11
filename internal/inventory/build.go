package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type offsets struct {
	file *token.File
}

func (o offsets) offset(pos token.Pos) int {
	return o.file.Offset(pos)
}

func (o offsets) line(pos token.Pos) int {
	return o.file.PositionFor(pos, false).Line
}

func (o offsets) column(pos token.Pos) int {
	return o.file.PositionFor(pos, false).Column
}

func (o offsets) positionForOffset(offset int) Position {
	p := o.file.PositionFor(o.file.Pos(offset), false)
	return Position{Line: p.Line, Column: p.Column}
}

// Build reads the Go source file at path and returns its inventory. The
// file must be gofmt-clean, not generated, and not a cgo file.
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

// BuildSource builds the inventory for src in memory. path and resolved
// are recorded verbatim as Document.File and Document.ResolvedFile.
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
	off := offsets{file: tokFile}

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
	previousLine := off.line(decls[0].Pos())
	for _, decl := range decls[1:] {
		line := off.line(decl.Pos())
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
		return ownedLineEnd(src, lastImport.End(), file.Comments, off)
	}
	return ownedLineEnd(src, file.Name.End(), file.Comments, off)
}

func fixedPostambleStart(src []byte, lastPost ast.Decl, comments []*ast.CommentGroup, off offsets) int {
	return ownedLineEnd(src, lastPost.End(), comments, off)
}

func buildEntities(src []byte, file *ast.File, decls []ast.Decl, preambleEnd, postambleStart int, off offsets) ([]Entity, error) {
	entities := make([]Entity, 0, len(decls))
	seenIDs := map[string]struct{}{}
	ordinals := map[string]int{}

	start := preambleEnd
	for i, decl := range decls {
		end := postambleStart
		if i < len(decls)-1 {
			end = ownedLineEnd(src, decl.End(), file.Comments, off)
		}
		if start < 0 || end < start || end > len(src) {
			return nil, fmt.Errorf("invalid segment bounds for entity %d", i)
		}
		declStart := off.offset(decl.Pos())
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

func ownedLineEnd(src []byte, endPos token.Pos, comments []*ast.CommentGroup, off offsets) int {
	ownedEnd := off.offset(endPos)
	ownedEndLine := off.line(endPos)
	end := lineEndAfter(src, ownedEnd)

	for _, group := range comments {
		groupStart, groupEnd := commentGroupBounds(group, off)
		if groupStart < ownedEnd {
			continue
		}
		if off.line(group.Pos()) != ownedEndLine {
			continue
		}
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
		Start:     off.positionForOffset(start),
		End:       off.positionForOffset(end),
	}
}

func countCommentsInRange(groups []*ast.CommentGroup, start, end int, off offsets) int {
	count := 0
	for _, group := range groups {
		groupStart, groupEnd := commentGroupBounds(group, off)
		if groupStart >= start && groupEnd <= end {
			count++
		}
	}
	return count
}

func commentGroupsInRange(groups []*ast.CommentGroup, start, end int, off offsets) []CommentGroup {
	out := []CommentGroup{}
	for _, group := range groups {
		groupStart, groupEnd := commentGroupBounds(group, off)
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

func commentGroupBounds(group *ast.CommentGroup, off offsets) (int, int) {
	return off.offset(group.Pos()), off.offset(group.End())
}

func rawCommentText(group *ast.CommentGroup) string {
	parts := make([]string, 0, len(group.List))
	for _, comment := range group.List {
		parts = append(parts, comment.Text)
	}
	return strings.Join(parts, "\n")
}

func importsC(file *ast.File) bool {
	for _, imp := range file.Imports {
		if imp.Path != nil && imp.Path.Value == `"C"` {
			return true
		}
	}
	return false
}
