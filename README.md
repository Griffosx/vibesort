# Vibesort

Vibesort is an opinionated tool for reorganizing declarations inside source files without letting an LLM rewrite source code directly.

The core idea is simple:

```text
LLM proposes declaration order.
Deterministic tooling validates and applies that order.
Formatter and checks verify the result.
```

Vibesort is deliberately conservative. It only accepts files that have already been normalized by the language formatter. For Go, that means the file must be `gofmt`-clean before Vibesort inventories, validates, or rewrites it. Non-formatted files are rejected instead of being silently cleaned during planning.

## Status

This repository currently contains the deterministic Go inventory core.

Implemented:

- `vibesort inventory file.go`
- Go source parsing with `go/parser`
- fixed preamble inventory
- post-preamble top-level entity inventory
- movable/pinned classification
- formatter-clean input checks
- generated file and cgo rejection
- identity round-trip verification
- in-memory complete-order reassembly checks
- inventory JSON output with a ready-to-edit order plan

Planned next:

- order-plan validation
- safe rewrite/apply
- post-rewrite formatting and verification
- skill wrappers for agent workflows

## Why

Large source files often drift into a hard-to-read order: handlers mixed with request types, public methods separated from helpers, or related functions scattered across unrelated code.

Vibesort aims to make those files easier to review without introducing hand-edited rewrites or broad formatting churn.

It is not a universal style enforcer. It is a fail-closed workflow for one narrow job: reorder declarations when the language adapter can prove the move is safe.

## How It Works

The intended workflow is:

```text
1. Run the language formatter.
2. Inventory the formatted file.
3. Ask the LLM for a structured order plan.
4. Validate the plan against the real source inventory.
5. Apply the reorder mechanically.
6. Format and verify the rewritten file.
7. Review the diff.
```

The LLM returns JSON, not source code:

```json
{
  "file": "internal/api/handlers/asset.go",
  "order": [
    {"id": "type:AssetHandler"},
    {"id": "func:NewAssetHandler"},
    {"id": "method:AssetHandler.Create"},
    {"id": "method:AssetHandler.List"}
  ]
}
```

Language detection and adapter choice are controlled by Vibesort, not by the LLM plan.

## Usage

Inventory a Go file:

```sh
go run ./cmd/vibesort inventory path/to/file.go
```

The command prints structured JSON describing the fixed preamble, top-level entities, movability, pinned reasons, source spans, comments, and the current valid order.

## Non-Goals

Vibesort should not:

- rewrite source code from raw LLM output
- accept unformatted input
- reorder every declaration type by default
- enforce one ordering style across all languages
- run automatically on every save
- replace formatters, linters, or dead-code tools
- create broad formatting-only diffs
