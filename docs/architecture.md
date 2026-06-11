# Vibesort Architecture

This document keeps the technical decisions out of the top-level README.

## Core Principle

Vibesort separates subjective organization from source mutation:

```text
LLM chooses an order.
Language adapter validates the order.
Language adapter rewrites mechanically.
Formatter normalizes the result.
Checks verify behavior.
```

The LLM must never return rewritten source as the artifact that gets applied.

## Formatter-Clean Input

Vibesort treats the language formatter as the input contract.

For Go, the source file must be `gofmt`-clean before inventory, validation, or apply. The tool should reject non-formatted files with a clear error. It should not silently format before planning, because that can hide unrelated formatting churn inside a reorder request.

After apply, Vibesort formats the rewritten output and verifies it again.

This gives the pipeline two formatter boundaries:

- before Vibesort: source must already be canonical
- after rewrite: output is canonical before verification and review

## Fail-Closed Safety

Vibesort should refuse when it cannot prove a move is safe.

Before rewrite:

- validate plan JSON
- validate declaration IDs against inventory
- reject unknown, duplicate, or missing IDs
- reject attempts to move pinned declarations
- reject unsupported files
- reject non-formatter-clean files
- reject risky constructs the adapter cannot safely model

After rewrite:

- format the output
- parse the output again
- inventory the output again
- compare declaration inventory before and after
- verify pinned relative order
- verify comments and entity content are preserved
- run language-specific static checks where practical
- show the diff

## Language Adapters

The orchestrator is language-agnostic. Each adapter owns language-specific semantics:

- declaration inventory
- plan validation
- source segmentation
- rewrite implementation
- formatter invocation
- post-rewrite verification

The orchestrator should infer the language from the selected file and adapter parser. The LLM plan must not choose or override the language.

## Go Adapter

Go is the first target because top-level functions and methods are usually reorderable.

Recommended implementation tools:

- `go/parser`
- `go/ast`
- `go/token`
- `go/printer`
- possibly `github.com/dave/dst` when comment preservation needs stronger source mapping

Safe v1 scope:

- inventory every top-level post-preamble declaration
- move top-level functions
- move methods
- move standalone type declarations only when safe
- pin package-level `var`
- pin package-level `const`
- pin `init`
- pin grouped declarations
- pin risky directive-bearing declarations

Avoid in v1:

- moving imports
- moving package docs
- moving generated files
- moving package-level initialization constructs
- splitting grouped declarations
- accepting files that are not `gofmt`-clean

Suggested Go verification:

```text
gofmt
go vet
go test
golangci-lint, when available
```

This repository does not currently define task or linter wiring. Add those only after the deterministic core is stable enough to justify project-level checks.

## Elixir Adapter

Elixir is possible, but stricter than Go.

Recommended implementation tools:

- Elixir parser and formatter
- `Code.string_to_quoted`
- `Macro`
- Sourceror for source-aware transformations and comment preservation

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

Suggested Elixir verification:

```text
mix format
mix compile
mix test
```

## Skill Layout

Vibesort is intended to be usable from agent skills while sharing one deterministic core.

Proposed layout:

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

`SKILL.md` should stay short. Detailed language rules should live in adapter references or adapter code.

## Future Languages

Future support should come through adapters, not one universal rewriter.

Possible adapters:

- Python: `libcst`
- TypeScript/JavaScript: TypeScript compiler API or `ts-morph`
- Rust: Rust parser plus `rustfmt`
- Java/C#: language-native parser or mature refactoring libraries

Tree-sitter can help with inventory and broad parsing, but it should not be the only rewrite layer unless the supported move is very constrained. Comment preservation and formatting are usually the hard parts.

## Related Tools

Vibesort is not a dead-code tool.

Recommended stance:

```text
Use staticcheck/unused in normal linting.
Use deadcode as a manual investigation tool.
Use Vibesort only for source organization.
```

## Rollout

Vibesort should not become a blocking CI step until the baseline is trusted.

Recommended rollout:

```text
1. Manual use on one file.
2. Manual use on one package.
3. Add a non-blocking task for local audits.
4. Consider CI only after the baseline is clean and the team trusts the output.
```

Possible future tasks:

```text
task vibesort -- path/to/file.go
task vibesort-check -- path/to/file.go
```
