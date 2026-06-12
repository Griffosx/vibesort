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

This repository currently contains the deterministic Go inventory and validation core.

Implemented:

- `vibesort inventory file.go`
- `vibesort validate plan.json`
- Go source parsing with `go/parser`
- fixed preamble inventory
- post-preamble top-level entity inventory
- movable/pinned classification
- formatter-clean input checks
- generated file and cgo rejection
- identity round-trip verification
- in-memory complete-order reassembly checks
- inventory JSON output with a ready-to-edit order plan
- strict order-plan validation

Planned next:

- safe rewrite/apply
- post-rewrite formatting and verification
- skill wrappers for agent workflows

Longer-term direction — ordering policy files, a CI check mode, within-package file moves — lives in [docs/direction.md](docs/direction.md).

## Why

Two reasons, one mechanical and one strategic.

The mechanical one: some files are catalogs, not narratives. Repositories, API clients, and handler sets are flat lists of similar-shaped declarations with no internal call flow, read as references — "does a method that does X already exist?" In those files physical order is the table of contents: grouped methods (for example CRUD) make the API surface scannable, and near-duplicates collide visually instead of accumulating. Narrative code — business logic with a call-graph story — gains nothing from sorting, and Vibesort is not meant for it.

The strategic one: LLMs and coding agents are good at deciding how code should be organized and unreliable at rewriting files without collateral damage. Vibesort splits the job: the model emits a plan over verified entity IDs, and deterministic tooling proves the result is a pure reorder — same bytes, same comments, new order. The safety harness is the product; reordering is the first edit small enough to prove safe.

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
  "schemaVersion": 1,
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

Validate an edited order plan:

```sh
go run ./cmd/vibesort validate plan.json
```

The command checks the plan against the current source file and makes no source changes.

## Trade-offs

Two costs are inherent and worth knowing up front:

- A pure reorder is a large moved-lines diff. Keep reorders in their own commits, separate from behavior changes, and consider listing them in `.git-blame-ignore-revs`.
- Comment attachment is verified mechanically; comment meaning is not. A comment like "the three helpers below handle retries" travels with one declaration and can become stale after a reorder. Review narrative comments in the diff.

## Non-Goals

Vibesort should not:

- rewrite source code from raw LLM output
- accept unformatted input
- reorder every declaration type by default
- enforce one ordering style across all languages
- run automatically on every save
- replace formatters, linters, or dead-code tools
- create broad formatting-only diffs
