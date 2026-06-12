package inventory

import (
	"bytes"
	"errors"
	"fmt"
)

// Reassemble returns the document's source with entities arranged in the
// given order. order must contain every entity ID exactly once, and pinned
// entities must keep their original relative order. doc must come from
// Build or BuildSource in the same process; documents decoded from JSON
// have no source segments and are rejected.
func Reassemble(doc *Document, order []string) ([]byte, error) {
	if err := validateReassembleSegments(doc); err != nil {
		return nil, err
	}
	byID, err := validateOrder(doc.Entities, order)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.Write(doc.Preamble.segment)

	var previousSegment []byte

	for _, id := range order {
		entity := byID[id]
		writeEntityBoundary(&out, previousSegment, entity.segment)
		out.Write(entity.segment)
		previousSegment = entity.segment
	}
	if doc.Postamble != nil {
		out.Write(doc.Postamble.segment)
	}

	return out.Bytes(), nil
}

// ValidateOrder checks that order is a complete, safe permutation of entities.
// Every entity ID must appear exactly once, and pinned entities must keep their
// original relative order.
func ValidateOrder(entities []Entity, order []string) error {
	_, err := validateOrder(entities, order)
	return err
}

func validateOrder(entities []Entity, order []string) (map[string]Entity, error) {
	if len(order) != len(entities) {
		return nil, fmt.Errorf("order length %d does not match entity count %d", len(order), len(entities))
	}

	seen := make(map[string]struct{}, len(order))
	for _, id := range order {
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate entity id %q", id)
		}
		seen[id] = struct{}{}
	}

	byID := make(map[string]Entity, len(entities))
	for _, entity := range entities {
		byID[entity.ID] = entity
	}

	pinnedOrder := []string{}
	for _, id := range order {
		entity, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown entity id %q", id)
		}
		if !entity.Movable {
			pinnedOrder = append(pinnedOrder, entity.ID)
		}
	}

	if !sameStrings(pinnedOrder, originalPinnedOrder(entities)) {
		return nil, errors.New("pinned entity relative order changed")
	}
	return byID, nil
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
	// Inventory-built segments normally end on line boundaries. This keeps
	// Reassemble defensive for tests or future callers with synthetic segments.
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
