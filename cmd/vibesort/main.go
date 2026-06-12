package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"vibesort/internal/inventory"
	"vibesort/internal/plan"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	switch args[0] {
	case "inventory":
		if len(args) != 2 {
			fmt.Fprintln(stderr, inventoryUsage)
			return 2
		}
		return runInventory(args[1], stdout, stderr)
	case "validate":
		if len(args) != 2 {
			fmt.Fprintln(stderr, validateUsage)
			return 2
		}
		return runValidate(args[1], stdout, stderr)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

func runInventory(path string, stdout, stderr io.Writer) int {
	doc, err := inventory.Build(path)
	if err != nil {
		fmt.Fprintf(stderr, "vibesort: %v\n", err)
		return 1
	}

	if err := encodeJSON(stdout, doc); err != nil {
		fmt.Fprintf(stderr, "vibesort: encode inventory: %v\n", err)
		return 1
	}
	return 0
}

func runValidate(path string, stdout, stderr io.Writer) int {
	p, err := plan.Load(path)
	if err != nil {
		fmt.Fprintf(stderr, "vibesort: %v\n", err)
		return 1
	}
	result, err := plan.Validate(p)
	if err != nil {
		fmt.Fprintf(stderr, "vibesort: %v\n", err)
		return 1
	}

	if err := encodeJSON(stdout, result); err != nil {
		fmt.Fprintf(stderr, "vibesort: encode validation result: %v\n", err)
		return 1
	}
	return 0
}

func encodeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

const inventoryUsage = "usage: vibesort inventory file.go"
const validateUsage = "usage: vibesort validate plan.json"
const usage = inventoryUsage + "\n       vibesort validate plan.json"
