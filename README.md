# Vibesort

## What It Is

Vibesort is a proposed skill and tool workflow for reorganizing declarations inside source files.

The core idea is:

```text
LLM decides the intended declaration order.
Deterministic language tooling applies and verifies the reorder.
```

The LLM should not rewrite source code directly. It should only produce a structured order plan. A language-specific adapter should validate that plan against the real source AST/CST, apply the reorder mechanically, run formatters, and run static checks.

Vibesort is intended to exist as both:

- a Codex skill
- a Claude Code skill

Both skills should share the same underlying scripts and validation logic so the behavior stays consistent across agents.

## Aim

The aim is to make large files easier to read without introducing risky hand-edits or noisy rewrites.

Vibesort should help with files where declarations have drifted into an unclear order, for example:

- handlers with request types, route methods, and helpers mixed together
- services with input structs, public methods, and private validation helpers scattered around
- modules where related functions are separated by unrelated code

The goal is not to enforce one universal ordering style across every language. The goal is to provide a repeatable, reviewable workflow for improving file organization while preserving behavior.

## Design Principles

### LLM as Planner, Not Editor

The LLM may choose an order, but it must not rewrite the source file.

Good:

```json
{
  "file": "internal/api/handlers/asset.go",
  "language": "go",
  "order": [
    {"id": "type:AssetHandler"},
    {"id": "func:NewAssetHandler"},
    {"id": "method:AssetHandler.Create"},
    {"id": "method:AssetHandler.List"}
  ]
}
```

Bad:

```text
LLM returns a rewritten Go file.
```

This split keeps subjective organization decisions with the LLM, while keeping source modification deterministic and auditable.

### Static Validation First

The LLM output must be validated before any file is changed.

The validator should reject:

- invalid JSON
- unknown files
- unknown declaration IDs
- duplicate declaration IDs
- missing required movable declarations, unless explicitly allowed
- attempts to move pinned declarations
- generated files
- files with risky directives unless allowlisted
- language-specific unsafe moves

### Language-Specific Safety

The workflow should be language-agnostic at the top level, but language-specific underneath.

Each language adapter owns:

- declaration inventory
- plan schema
- safety validation
- source rewrite
- formatting
- verification commands

The orchestrator should not assume that all languages have Go-like declaration semantics.

## How It Works

The proposed workflow:

```text
1. User asks Vibesort to organize one or more files.
2. Orchestrator detects the language.
3. Language adapter extracts movable declarations from the source file.
4. LLM receives the inventory and ordering policy.
5. LLM returns a structured declaration order plan.
6. Validator checks the plan against the AST/CST inventory and safety rules.
7. Language adapter applies the reorder mechanically.
8. Formatter runs.
9. Static checks and tests run.
10. User reviews the diff.
```

The most important invariant:

```text
AST/CST content should remain the same except for declaration order.
```

## Proposed Skill Layout

```text
vibesort/
  SKILL.md
  scripts/
    vibesort.py
    adapters/
      go/
        inventory.go
        reorder.go
        validate.go
      elixir/
        inventory.exs
        reorder.exs
        validate.exs
  references/
    policy.md
    go.md
    elixir.md
```

`SKILL.md` should stay short. It should explain when to use Vibesort, the safe workflow, and which script to call.

Detailed language rules should live in references or adapter code.

## Orchestrator

A Python orchestrator is reasonable because it can coordinate:

- file selection
- language detection
- invoking language adapters
- invoking Codex or Claude Code
- validating JSON output
- running formatters and checks

The orchestrator should not parse and rewrite every language itself. It should delegate that to language adapters.

## Go Adapter

Go is a good first target because declaration order is usually not semantically meaningful for functions and methods.

Recommended implementation:

- use `go/parser`, `go/ast`, `go/printer`, and `go/token`
- consider `github.com/dave/dst` for better comment preservation
- apply `gofumpt` and `goimports` after rewriting

Safe v1 scope:

- reorder top-level functions
- reorder methods
- optionally reorder type declarations only when explicitly enabled

Avoid in v1:

- package docs
- imports
- package-level `var`
- package-level `const`
- `init` functions
- generated files
- declarations with `//go:embed`, `//go:generate`, `//go:linkname`, or similar directives

Suggested Go ordering policies:

For handlers:

```text
request/response types
handler struct
constructor
public handlers grouped by resource and CRUD flow
private parsing helpers
private response helpers
```

For services:

```text
input/output types
service struct
constructor
public service methods
private validation helpers
private normalization/build helpers
```

Verification for Go:

```text
gofumpt
goimports
golangci-lint
go vet
go test
```

In this repository, `task check-all` already runs `golangci-lint`, and `.golangci.yml` already enables `staticcheck` and `unused`. A separate `staticcheck` task may be useful for focused local checks, but it does not need to be added to `check-all`.

## Elixir Adapter

Elixir is possible, but stricter than Go.

Recommended implementation:

- use Elixir tooling for parsing and rewriting
- use `Code.string_to_quoted` and `Macro` for AST basics
- prefer Sourceror for source-aware transformations and comment preservation
- run `mix format` after rewriting

Important Elixir risks:

- `@doc`, `@spec`, and `@impl` attach to the next function
- multiple clauses of the same function must stay together
- clause order can change pattern matching behavior
- module body code runs at compile time
- macros and DSLs can be order-sensitive
- Phoenix router files are order-sensitive
- Ecto schema blocks should not be freely reordered
- test `describe` and `setup` ordering can matter

Safe v1 scope:

- reorder normal `def`, `defp`, `defmacro`, and `defmacrop` blocks
- move attached docs, specs, impl attributes, and comments with the function
- keep all clauses of the same function together
- preserve internal clause order

Avoid in v1:

- `use`, `import`, `alias`, `require`
- general module attributes
- router files
- Ecto schema blocks
- test setup blocks
- macros or DSL-heavy modules unless explicitly allowlisted

Verification for Elixir:

```text
mix format
mix compile
mix test
```

## Future Language Support

Vibesort should support future languages through adapters, not through one universal rewriter.

Possible adapters:

- Python: `libcst`
- TypeScript/JavaScript: TypeScript compiler API or `ts-morph`
- Rust: Rust parser plus `rustfmt`
- Java/C#: language-native parser or mature refactoring libraries

Tree-sitter can be useful for inventory and broad parsing, but it should not be the only rewrite layer unless the supported move is very constrained. Comment preservation and formatting are usually the hard parts.

## Static Checks

Vibesort should have two layers of static checking.

Before rewrite:

- validate the LLM JSON schema
- validate declaration IDs against inventory
- validate all safety rules
- reject unsupported files or risky constructs

After rewrite:

- format the file
- parse the file again
- compare declaration inventory before and after
- run language-specific static checks
- run tests where practical
- show the diff

The tool should fail closed. If it is unsure whether a move is safe, it should refuse and explain why.

## Dead Code And Related Tools

Vibesort is not a dead-code tool.

Related tools discussed:

- `staticcheck`: widely used Go static analysis tool
- `unused`: Go unused-code linter, already enabled in this repo through `golangci-lint`
- `deadcode`: useful advisory tool for Go call-graph dead-code audits

Recommended stance:

```text
Use staticcheck/unused in normal linting.
Use deadcode as a manual investigation tool.
Use Vibesort only for source organization.
```

## CI Strategy

Vibesort should not be added as a blocking `check-all` step initially.

Recommended rollout:

```text
1. Manual use on one file.
2. Manual use on one package.
3. Add a non-blocking task for local audits.
4. Consider CI only after the baseline is clean and the team trusts the output.
```

The first useful task could be:

```text
task vibesort -- path/to/file.go
```

Later:

```text
task vibesort-check -- path/to/file.go
```

`vibesort-check` would verify whether a file matches the expected order without rewriting it.

## Non-Goals

Vibesort should not:

- rewrite source code with raw LLM output
- reorder every declaration type by default
- enforce one universal style across all languages
- run automatically on every save
- create massive formatting-only diffs
- replace formatters, linters, or dead-code tools

## Summary

Vibesort is an LLM-guided, tool-verified declaration ordering workflow.

The safest model is:

```text
LLM proposes order.
Adapter validates order.
Adapter rewrites source.
Formatter normalizes source.
Static checks verify behavior.
Human reviews diff.
```

Start with Go functions and methods only. Add Elixir later with stricter rules. Keep the orchestration language-agnostic, but keep all parsing and rewriting language-specific.
