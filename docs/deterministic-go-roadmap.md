# Vibesort Deterministic Go Roadmap

Build the deterministic Go core before adding any LLM calls or skill wrappers.

The core accepts a JSON order plan, validates it against the real Go source file, and eventually rewrites the file so post-preamble top-level entities appear in the requested order. The implementation must stay deterministic, reviewable, and fail-closed.

## Core Model

All sprints share one model.

Vibesort operates on one formatter-clean Go source file at a time. Non-`gofmt` input is rejected instead of silently formatted, so reorder diffs do not hide unrelated formatting churn.

The file is partitioned into:

```text
fixed preamble      package docs, package clause, imports, import-owned comments
entities            every post-preamble top-level declaration
fixed postamble     trailing whitespace and loose EOF comments
```

The segments must tile the original file without gaps or overlaps. Reassembling the preamble, entities in current order, and postamble must reproduce the original bytes exactly.

The preamble and postamble are fixed anchors. They never appear in `plan.order`.

Floating comments after imports bind downward to the first entity. Floating comments between declarations also bind downward to the next entity. Trailing same-line comments belong to the entity on that line.

## Entity Model

Every post-preamble top-level construct is inventoried as one entity:

- package-level `const`
- package-level `var`
- standalone type declarations
- grouped declarations
- top-level functions
- methods
- `init` functions

Each entity has a stable ID, kind, source index, span, owned comments, movability, optional pinned reason, and human preview fields such as signature and first doc line.

Common IDs:

```text
type:AssetHandler
func:NewAssetHandler
method:AssetHandler.Create
var:0
const:0
init:0
type_group:0
```

Movable by default:

- top-level functions
- methods
- standalone type declarations

Pinned by default:

- package-level `var`
- package-level `const`
- `init`
- grouped declarations in v1
- blank identifier declarations
- directive-bearing declarations
- all entities in files affected by Go line directives

Pinned entities still appear in plans, but validation rejects plans that change their relative order.

## Order Plan Model

The LLM, if used later, proposes JSON, not source code:

```json
{
  "file": "internal/api/handlers/asset.go",
  "order": [
    {"id": "type:AssetHandler"},
    {"id": "func:NewAssetHandler"},
    {"id": "method:AssetHandler.Create"}
  ]
}
```

The plan does not include language metadata, preamble IDs, postamble IDs, spans, comments, or rewritten source.

`order` must include every post-preamble entity exactly once.

Movable entities may move around pinned entities, but pinned entities must keep their original relative order:

```text
original: A, pinned:X, B, pinned:Y, C
valid:    C, pinned:X, A, B, pinned:Y
invalid:  A, pinned:Y, C, pinned:X, B
```

## Sprints

1. Inventory: build and prove the read-only inventory model.
2. Plan validation: add complete order-plan validation.
3. Apply and verify: add rewriting only after inventory and validation are trustworthy.

The risky part is not calling an LLM. The risky part is defining what can be moved without losing comments, changing behavior, or producing confusing diffs.

### Sprint 1: Inventory

Command:

```text
vibesort inventory file.go
```

Status: implemented.

Inventory prints structured JSON with the fixed preamble summary, every entity, owned comments, movability, pinned reasons, and a ready-to-edit order plan in current source order.

Required checks:

- target path is a `.go` file
- target file parses
- target file is `gofmt`-clean
- generated files are rejected
- cgo `import "C"` files are rejected
- identity round-trip passes
- Go directives conservatively pin affected entities

### Sprint 2: Plan Validation

Command:

```text
vibesort validate plan.json
```

Status: next target.

Validation parses the plan, inventories the current target file, and rejects invalid plans without changing the filesystem.

Detailed contract: [Plan Validation Spec](plan-validation-spec.md).

### Sprint 3: Apply And Verify

Command:

```text
vibesort apply plan.json
```

Status: not implemented.

Apply will run inventory and validation, reassemble entity segments in requested order, run `gofmt`, parse and inventory again, verify entity/comment/content preservation, then atomically write only after all checks pass.

## Current State

Sprint 1 is implemented.

Sprint 2 is the next implementation target.

## Out Of Scope For The Deterministic Go Core

- LLM calls
- Codex or Claude Code skill wrappers
- Elixir or other language adapters
- partial-order plans
- moving imports
- moving package-level `var`, `const`, or `init` by default
- splitting grouped declarations
- project-specific formatter detection
- `goimports` or `gofumpt` integration
- dead-code analysis
- automatic CI enforcement
