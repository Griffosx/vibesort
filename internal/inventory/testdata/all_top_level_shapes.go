//go:build fixtures
// +build fixtures

// Package fixtures exercises top-level declaration inventory shapes.
package fixtures

import (
	// blank import comment
	_ "embed"
	"fmt" // fmt import comment
)

// Alias doc.
type Alias = string

/* Reader doc block. */
type Reader interface {
	Read([]byte) (int, error)
}

// HandlerFunc doc.
type HandlerFunc func(string) error

// Pair doc.
type Pair[T any] struct {
	Left  T
	Right T
}

// Names doc.
var A, B = 1, 2

//go:embed all_top_level_shapes.go
var Embedded string

const (
	// First starts iota.
	First = iota
	Second
)

const Single = "single"

// Blocky is here.
func Blocky() {} /* trailing block
continues */

// Runner has a value receiver.
func (p Pair[T]) Run() {}

func init() {}

func init() {}
