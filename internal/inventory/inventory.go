// Package inventory builds a lossless, fail-closed model of the top-level
// declarations in a single gofmt-clean Go source file.
//
// The model partitions the source into a fixed preamble (package clause and
// imports), an ordered list of entities (the remaining top-level
// declarations), and an optional fixed postamble (trailing comments). Every
// byte belongs to exactly one segment, which Build proves by reassembling
// the segments in original order and requiring the result to match the
// input exactly.
//
// Files that cannot be modeled safely are rejected instead of approximated:
// sources that are not gofmt-clean, generated files, cgo files, and files
// with multiple top-level declarations on one line.
package inventory

// SchemaVersion identifies the JSON layout of Document. Consumers should
// reject documents carrying an unknown version.
const SchemaVersion = 1

// Document is the inventory of one Go source file. Its JSON form is the
// public contract consumed by plan-producing tools.
//
// The raw source segments are unexported, so a Document decoded from JSON
// cannot be reassembled; rebuild it from source instead.
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

// Preamble is the fixed segment from the start of the file through the
// package clause and imports. It never moves.
type Preamble struct {
	Kind         string `json:"kind"`
	Fixed        bool   `json:"fixed"`
	Span         Span   `json:"span"`
	CommentCount int    `json:"commentCount"`

	segment []byte
}

// Postamble is the fixed segment after the last declaration, present only
// when the file ends with trailing comments. It never moves.
type Postamble struct {
	Kind         string `json:"kind"`
	Fixed        bool   `json:"fixed"`
	Span         Span   `json:"span"`
	CommentCount int    `json:"commentCount"`

	segment []byte
}

// Entity is one post-preamble top-level declaration together with the
// comment groups it owns.
//
// IDs follow a fixed grammar: "func:Name", "method:Receiver.Name", and
// "type:Name" for uniquely named declarations; ordinal forms such as
// "var:0", "const:0", "init:0", and "type_group:0" otherwise. Movable
// reports whether a plan may reorder the entity. Pinned entities state why
// in PinnedReason and must keep their original relative order.
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

// Span locates a segment as the half-open byte range [StartByte, EndByte)
// with the corresponding line/column positions.
type Span struct {
	StartByte int      `json:"startByte"`
	EndByte   int      `json:"endByte"`
	Start     Position `json:"start"`
	End       Position `json:"end"`
}

// Position is a 1-based line and column in the source file.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// CommentGroup is the raw text and location of one owned comment group.
type CommentGroup struct {
	Text string `json:"text"`
	Span Span   `json:"span"`
}

// ReadyPlan is the current entity order in plan form, ready to be permuted
// by a plan-producing tool and fed back for validation.
type ReadyPlan struct {
	File  string      `json:"file"`
	Order []OrderItem `json:"order"`
}

// OrderItem references one entity by ID.
type OrderItem struct {
	ID string `json:"id"`
}
