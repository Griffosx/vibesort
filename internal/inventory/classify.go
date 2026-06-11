package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
)

const blankIdentifierPinnedReason = "blank identifier declaration has no stable name"
const lineDirectivePinnedReason = "go directive: line directive"

var recognizedGoDirectives = map[string]struct{}{
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
		case token.VAR, token.CONST:
			kind := d.Tok.String()
			entity.Kind = kind
			entity.ID = ordinalID(kind, ordinals)
			entity.PinnedReason = "package " + kind + " order can be significant"
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
	declStart := off.offset(decl.Pos()) - segmentStart
	if declStart < 0 || declStart >= len(segment) {
		return ""
	}
	return strings.TrimSpace(string(segment[declStart:lineEndAfter(segment, declStart)]))
}

func directiveCommentsForDecl(decl ast.Decl, start, end int, groups []*ast.CommentGroup, off offsets) []*ast.Comment {
	declStart := off.offset(decl.Pos())
	declEnd := off.offset(decl.End())
	comments := []*ast.Comment{}
	seen := map[*ast.Comment]struct{}{}

	addGroup := func(group *ast.CommentGroup) {
		if group == nil {
			return
		}
		groupStart, groupEnd := commentGroupBounds(group, off)
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
		groupStart, groupEnd := commentGroupBounds(group, off)
		if groupStart < start || groupEnd > end {
			continue
		}
		if groupEnd <= declStart {
			addGroup(group)
			continue
		}
		if groupStart >= declEnd && off.column(group.Pos()) == 1 {
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
			if off.offset(comment.Slash) >= end {
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
	for _, comment := range comments {
		line := comment.Text
		if isLineDirective(comment, off) {
			return "line directive"
		}
		if !strings.HasPrefix(line, "//go:") {
			continue
		}
		name := strings.TrimPrefix(line, "//")
		if fields := strings.Fields(name); len(fields) > 0 {
			name = fields[0]
		}
		if name == "go:generate" && off.column(comment.Slash) != 1 {
			continue
		}
		if _, ok := recognizedGoDirectives[name]; ok {
			return name
		}
		return "unknown " + name
	}

	return ""
}

func isLineDirective(comment *ast.Comment, off offsets) bool {
	text := comment.Text
	switch {
	case strings.HasPrefix(text, "//line "):
		return off.column(comment.Slash) == 1 && strings.Contains(text, ":")
	case strings.HasPrefix(text, "/*line "):
		return strings.HasSuffix(text, "*/") && strings.Contains(text, ":")
	default:
		return false
	}
}
