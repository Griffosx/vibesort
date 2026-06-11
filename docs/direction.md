# Vibesort Direction

Status: agreed direction, not a sprint commitment. Sprints 1–3 in the
[roadmap](deterministic-go-roadmap.md) come first.

This document records why the project exists, where it should and should not
be used, and the design decisions for what comes after the deterministic core.

## The Product Is the Harness

Vibesort's pattern is constrained LLM editing: the model proposes a structured
plan over verified entity IDs, and deterministic tooling validates and applies
it. This eliminates whole failure classes by construction — hallucinated code,
dropped comments, behavior edits hidden inside a large diff.

Reordering declarations is the first edit small enough to prove safe with an
identity round-trip. It is the demonstration vehicle, not the destination. The
durable value is the verification primitive: prove that an edit is a pure
permutation of verified source segments. A plain text diff cannot give that
guarantee, and it matters most in agent workflows, where nobody reads every
generated diff line by line.

## Where Ordering Pays

Code splits into catalogs and narratives.

Catalog files — repositories, stores, API clients, handler sets — are flat
lists of similar-shaped declarations with no internal call flow. They are read
as references: "does a method that does X already exist?" There, physical
order is the table of contents, grouped ordering (for example CRUD) makes the
API surface scannable, and adjacency makes near-duplicates collide visually
instead of accumulating.

Narrative files — business logic with a call-graph story — gain nothing from
sorting. Reordering them is churn. Any ordering policy must be scoped to the
files it helps; an unscoped "sort everything" mode would be net negative and
must not exist.

Ordering conventions also decay: unenforced, they lose to append-at-the-bottom
entropy within months. A convention is only worth adopting if a machine keeps
it true. That is why the check mode below matters more than the apply mode.

## Ordering Policy: the `.vibesort` File

Planned design, not yet scheduled.

A repository declares how its catalog files are ordered:

```yaml
match: "**/repository/*.go"
groups:
  - create: [Create*, Add*, Insert*, Upsert*]
  - read:   [Get*, Find*, List*, Count*, Exists*]
  - update: [Update*, Set*]
  - delete: [Delete*, Remove*, Archive]
within: alphabetical
unmatched: fail
```

Design rules:

- The policy is structured and deterministic at runtime. Same policy, same
  source, same result — on every machine and every model version.
- The LLM is a policy author, not a policy interpreter. It may propose the
  file, extend it, and classify a method the patterns miss — but each decision
  is written back into the policy and reviewed, never re-derived per run.
- `match` scoping is mandatory. Policies apply to declared paths only.
- `vibesort check` makes the policy enforceable in CI with readable failures
  ("unclassified: GetOrCreate — add a rule or an override").

Every ordering enforcement that has survived in practice — StyleCop member
ordering, IDE arrangement rules, import sorters — is a deterministic rule
engine. Prose interpreted by a model at run time cannot fail a pull request
coherently, and its meaning drifts when models change. Vibesort does not put
an LLM in the enforcement loop.

## After the Core: Depth Over Breadth

In rough order of value:

1. Check mode. `vibesort check` against a `.vibesort` policy. Turns the tool
   from a once-a-year cleanup into something CI runs on every pull request,
   and prevents convention decay.
2. Within-package file moves. In Go, declarations are package-scoped, so
   moving a declaration between files of the same package is semantically
   neutral and mechanically checkable with the existing inventory model (plus
   per-file import fixing). "Split this 2,000-line file, with proof nothing
   changed" addresses a far stronger pain than within-file ordering.
3. More languages, later. The Elixir adapter and other languages stay design
   references until the Go adapter has proven the workflow end to end. A
   second language doubles the surface while serving the same problem;
   cross-file Go serves a bigger problem with machinery that already exists.

## Known Limits

- A pure reorder is a large moved-lines diff and disturbs `git blame`.
  Mitigation is workflow, not code: reorder-only commits plus
  `.git-blame-ignore-revs`.
- Comment attachment is proven mechanically; comment meaning is not. A
  comment describing "the next three functions" follows one declaration and
  can become stale. No reordering tool can solve this; Vibesort names it
  instead of implying otherwise.
