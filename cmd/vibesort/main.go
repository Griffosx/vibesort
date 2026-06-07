package main

import (
	"encoding/json"
	"fmt"
	"os"

	"vibesort/internal/inventory"
)

func main() {
	if len(os.Args) != 3 || os.Args[1] != "inventory" {
		fmt.Fprintln(os.Stderr, "usage: vibesort inventory file.go")
		os.Exit(2)
	}

	doc, err := inventory.Build(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "vibesort: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		fmt.Fprintf(os.Stderr, "vibesort: encode inventory: %v\n", err)
		os.Exit(1)
	}
}
