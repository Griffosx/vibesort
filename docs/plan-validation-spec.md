# Vibesort Plan Validation Spec

Status: implemented for Sprint 2.

This spec defines the read-only validation contract for:

```text
vibesort validate plan.json
```

The command decides whether a complete declaration order plan is safe for the current contents of a Go source file. It must not modify the filesystem.

## Scope

In scope:

- parse and validate plan JSON
- inventory the referenced Go source file
- enforce complete entity coverage
- reject unknown, duplicate, or missing IDs
- enforce pinned entity relative order
- return clear failures without writing files

Out of scope:

- rewriting source files
- accepting inventory JSON as input
- accepting partial-order plans
- letting the plan choose a language adapter
- formatting or fixing the target file
- validating subjective grouping preferences from an LLM

## Command Contract

```text
vibesort validate plan.json
```

Exit codes:

- `0`: plan is valid
- `1`: plan or target file is invalid
- `2`: command-line usage error

Valid plans print one JSON object to stdout:

```json
{
  "valid": true,
  "file": "internal/api/handlers/asset.go",
  "entityCount": 4
}
```

Invalid plans print one concise error to stderr:

```text
vibesort: duplicate entity id "func:CreateAsset"
```

Usage errors print:

```text
usage: vibesort validate plan.json
```

## Plan Schema

```json
{
  "schemaVersion": 1,
  "file": "internal/api/handlers/asset.go",
  "order": [
    {"id": "type:AssetHandler"},
    {"id": "func:NewAssetHandler"},
    {"id": "method:AssetHandler.Create"}
  ]
}
```

Required fields:

- `schemaVersion`: integer plan schema version; this spec defines version `1`
- `file`: string path to one Go source file
- `order`: array of order items

The version lives on the plan itself because plans travel as standalone files; the inventory document's `schemaVersion` does not protect them. The inventory `readyPlan` output should gain the same field in Sprint 2 so it remains a valid plan as-is.

Inventory document `schemaVersion` is a compatibility version, not an exact-shape version. Additive inventory JSON fields do not require a bump; breaking semantic changes, renamed fields, removed fields, or field type changes do.

Order item fields:

- `id`: string entity ID from the current inventory

Unknown top-level plan fields and unknown order item fields should be rejected in Sprint 2. This catches misspelled generated fields early.

The plan must not include language metadata, formatter metadata, source spans, preamble IDs, postamble IDs, comments, or rewritten source text.

## Path Resolution

`file` is resolved relative to the current working directory of the `vibesort validate` process, unless it is already absolute.

The target file path is not implicitly relative to the plan file's directory in Sprint 2. This matches the current `inventory` command and avoids hidden cwd changes.

The target file must:

- have a `.go` extension
- exist
- parse as Go
- be `gofmt`-clean
- not be generated code
- not use cgo `import "C"`
- pass the inventory identity round-trip

## Validation Rules

Validation runs against a fresh inventory of the target file. Serialized inventory JSON is not trusted as input.

Rules:

1. Plan JSON must be syntactically valid.
2. Plan JSON must match the schema exactly.
3. `schemaVersion` must equal a supported version (currently `1`).
4. `file` must resolve to an inventory-supported Go source file.
5. `order` length must equal the number of post-preamble entities.
6. Every post-preamble entity ID must appear exactly once.
7. Unknown IDs are rejected.
8. Duplicate IDs are rejected.
9. Fixed preamble and postamble segments must not appear in `order`.
10. Pinned post-preamble entities must keep their original relative order.

Recommended error priority:

1. unreadable or malformed plan JSON
2. schema/type errors
3. unsupported `schemaVersion`
4. unreadable or unsupported target file
5. inventory failures
6. order length mismatch
7. duplicate IDs
8. unknown IDs
9. missing IDs
10. pinned relative-order change

This priority gives predictable tests. It is not a promise to report every issue in one run.

## Test Matrix

Sprint 2 should add tests for:

- valid identity order plan
- valid non-trivial movable reorder
- malformed JSON
- missing `schemaVersion`
- unsupported `schemaVersion`
- missing `file`
- missing `order`
- wrong field types
- unknown top-level field
- unknown order item field
- non-Go target file
- parse error in target file
- non-`gofmt` target file
- generated target file
- cgo target file
- duplicate entity ID
- unknown entity ID
- missing entity ID
- fixed preamble/postamble sentinel IDs rejected as unknown
- pinned relative order changed
- plan validated against current source, not stale inventory output

CLI smoke tests should cover valid plans, invalid plans, wrong argument count, and unknown subcommands.

## Implementation Notes

Reuse the inventory model directly instead of duplicating entity classification logic.

Recommended internal shape:

```text
internal/plan
  plan.go        JSON schema and decoding
  validate.go    source inventory plus order validation
```

The existing `inventory.Reassemble` function enforces several useful constraints, but Sprint 2 should expose validation as its own read-only concept. `Reassemble` needs source segments and is shaped around producing bytes; validation should explain plan failures without implying mutation.
