# Milestone 1 — Parser foundation (skeleton + parser)

Status: implemented · Estimate: **6 dev-days** (+ ~1 day for 2.1 syntax)
Depends on: nothing

> **Implementation note (2026-11-13).** The parser and fixture harness are
> implemented. The upstream corpus has moved on since this plan was written:
> it contains 19 valid files that declare 2.1 or use 2.1-only syntax (not 14),
> and the 2.1 grammar deltas are small and local. Rather than skip them, the
> implementation parses the 2.1 syntax additions — member indexes, inline
> collection targets, and tagged string literals — while leaving 2.1
> *semantics* to M2 (implemented later in Milestone 5). All 100 valid fixtures parse. 180 of 207 invalid
> fixtures match their expected parser error; the other 27 are semantic and
> are listed in `testdata/loader/CLASSIFICATION.md`.

Absorbs the old "M0 skeleton" and "M1 parser": the repository skeleton, fixture
harness, and parser are one unit of work. A parser without the harness cannot be
declared done; a skeleton without the parser delivers nothing.

---

## 1. Unit of work

Deliver a Go module that can read Smithy IDL **2.0** files and produce a
source-located AST, validated against the real Smithy fixture corpus. No
semantic resolution, no validation, no code generation.

**Demo**

```sh
$ go run ./cmd/gosmithy parse testdata/weather.smithy
{
  "version": "2.0",
  "namespace": "example.weather",
  ...
}
```

**Why this is one unit**

- The parser is only proven by running the fixture corpus; the harness is part
  of the parser deliverable.
- The CLI shell, module layout, and test conventions are decided by what the
  parser needs. Splitting them creates artificial handoffs.
- There is no useful intermediate artifact between "empty repo" and "parses the
  corpus".

---

## 2. Scope

### In

- Go module `github.com/tespeleta/gosmithy`.
- `cmd/gosmithy` with a `parse` command (plus `version` and `--help`).
- `idl` package:
  - tokenizer with exact Smithy whitespace behavior;
  - AST types for the full IDL 2.0 grammar;
  - parser with source locations and doc-comment capture;
  - version gate for 2.1 syntax;
  - 2.1 syntax: member indexes, inline collection targets, tagged literals.
- Fixture corpus copied from the Java repo:
  - `testdata/loader/valid/**` (100 `.smithy` files, 18 of which use 2.1
    syntax or declare a 2.1 version);
  - `testdata/loader/invalid/**` (207 `.smithy` files, classified as syntax
    vs semantic in `CLASSIFICATION.md`);
  - Apache-2.0 attribution.
- Test runner that loads fixtures and asserts parse success or expected error.
- Fuzz seed target for the tokenizer/parser.
- Small README for the CLI and test commands.

### Out

- Semantic model, shape resolution, trait resolution, validation.
- Code generation, plugins, protocols.
- AWS anything.
- JSON AST loader (the AST JSON emitted by `parse` is only a debug view).
- Formatting/unparsing.
- 2.1 semantic assembly: synthetic inline-collection shapes, synthetic names,
  and `idx` trait application. The M1 parser records the syntax; M2 resolves
  it. (2.1 syntax itself is implemented; see the note above.)

---

## 3. Grammar checklist (IDL 2.0 + 2.1 syntax)

The parser must handle the productions in `docs/source-2.0/spec/idl.rst`, with
these concrete checkpoints:

- **Lexical**
  - whitespace: spaces, tabs, newlines, `\r\n`, commas as whitespace;
  - `//` line comments, `///` doc comments;
  - single- and double-quoted? (Smithy only uses `"`); strings with escapes and
    `\uXXXX`;
  - text blocks (`"""`), including indentation stripping;
  - numbers (integer/decimal/exponent, negative);
  - identifiers, keywords (`true`, `false`, `null`), shape IDs, member IDs;
  - punctuation: `@ # $ { } [ ] ( ) : := = . ,`.
- **Control section**
  - `$version: "2.0"`; unknown control statements are warnings, not parse errors;
  - `$operationInputSuffix` / `$operationOutputSuffix` if present.
- **Metadata**
  - `metadata key = <node>`; quoted or unquoted keys.
- **Shape section**
  - `namespace a.b.c`, `use a.b#Shape`.
  - simple shapes: `blob`, `boolean`, `document`, `string`, numeric types,
    `timestamp`.
  - `enum` / `intEnum` with explicit and implicit values, traits, docs.
  - `list` / `map` / `structure` / `union`, members with traits and targets.
  - `service` / `resource` / `operation` with node-object bodies and typed
    properties (parse as node members; interpretation is M2).
  - operation inline input/output (`input := { ... }`, `output := { ... }`).
  - `with [Mixin]` on all shape kinds.
  - member elision `$foo`, member index `1. foo` (2.1 syntax).
  - inline collection targets `[T]` / `{K: V}` (2.1 syntax).
  - tagged string literals `#re`, `#b`, `#hex`, `#timestamp` (2.1 syntax).
- **Traits**
  - `@trait`, `@trait(...)`, `@trait(value)`, empty trait body;
  - trait bodies with structure shorthand (`@length(min: 1)`) vs a single node
    value;
  - `apply Shape @trait` and `apply Shape { @a @b }`.
- **Node values**
  - object, array, string, number, boolean, null;
  - unquoted identifiers are syntactic shape IDs (mark them for M2);
  - text blocks as values.
- **Doc comments**
  - consecutive `///` lines attach to the next shape/member/trait target;
  - doc comments before `apply` and at EOF behave per spec (bad-doc warning is
    M2, not a parser crash).

---

## 4. AST sketch

Keep the AST close to syntax, not semantics.

```go
package idl

type File struct {
    Path      string
    Version   Version
    Controls  map[string]Node
    Metadata  []Metadata
    Namespace string
    Uses      []Use
    Shapes    []Stmt // shape or apply
    Source    SourceLocation
}

type Stmt interface{ stmt() }

type ShapeStmt struct {
    Traits []Trait
    Kind   ShapeKind // simple, enum, intEnum, list, map, structure, ...
    Name   string
    Mixins []ShapeRef
    Members []Member
    Body   Node         // service/resource/operation properties
    Input, Output *InlineIO
    Errors []ShapeRef
    ForResource string  // 2.1? present in repo
    Source SourceLocation
    Doc    string
}

type Member struct {
    Traits []Trait
    Name   string
    Target string      // unresolved
    Value  *Node       // enum/intEnum value
    Elided bool
    Index  *int        // 2.1
    Source SourceLocation
    Doc    string
}

type Trait struct {
    Name   string      // unresolved
    Value  Node
    Source SourceLocation
}

type ApplyStmt struct {
    Target ShapeRef
    Traits []Trait
    Source SourceLocation
}
```

> The implemented AST refines this sketch: control and metadata statements are
> ordered slices, member targets are a `Target` value that can carry an inline
> collection, and `Doc`/`DocSource` fields preserve documentation comments.
> See `idl/ast.go` for the authoritative types.

Rules:

- All identifiers that refer to shapes stay strings until M2.
- Member shapes are not created here; M2 does that.
- Keep the original token span for every node and trait so M2 can report precise
  source locations.
- `Version` is `2.0` by default when no `$version` is present, matching the spec's
  "assume loadable" guidance. In MVP, a missing version parses as 2.0.

---

## 5. Work breakdown

| Unit | Work | Estimate |
|---|---|---|
| U1 | Repository skeleton, `go.mod`, CLI shell, `parse` command, fixture copy + attribution | 0.5d |
| U2 | Tokenizer: whitespace/comments/strings/text blocks/numbers/shape IDs; token spans | 1.5d |
| U3 | AST types + doc-comment capture + source locations | 0.5d |
| U4 | Parser: control, metadata, namespace/use, simple/aggregate shapes, service/resource/operation, traits, apply, inline I/O, mixins | 2.5d |
| U5 | Version gating: 2.0 vs 2.1 constructs; 2.1 syntax | 1d |
| U6 | Fixture harness: valid parse runner, invalid expected-error runner, syntax/semantic classification | 0.5d |
| U7 | Tests, fuzz seed, CLI output polish, docs | 0.75d |

### U1 details

- `go mod init github.com/tespeleta/gosmithy`
- `cmd/gosmithy/main.go`: `parse`, `version`, `--help`.
- `testdata/loader/...` copied from
  `smithy/smithy-model/src/test/resources/software/amazon/smithy/model/loader/`.
- `testdata/NOTICE` with Apache-2.0 attribution.
- `docs/plan` already exists; add a short `README.md`.

### U2 details

Follow the Smithy token model:

- whitespace tokens are significant for the parser's line-break rules;
- commas are whitespace;
- comments are whitespace; doc comments are a distinct token and are buffered;
- `//` followed by `/` is a doc comment, not a line comment;
- string escapes, unicode, text-block indentation;
- numbers: reject `1.` (must be followed by digit);
- tagged string literals: recognize `#tag "..."` and gate on 2.1.

### U4 details

Parse line breaks according to the ABNF (`BR`), not just "skip whitespace".
This is where most hand-written Smithy parsers get subtle bugs:

- after `namespace`, `use`, control, metadata, traits, members;
- optional commas before line breaks;
- doc comments are allowed between traits and shapes;
- a shape statement must be followed by a line break unless EOF.

### U6 details

- Valid runner: parse every `testdata/loader/valid/**/*.smithy` and assert no
  error.
- Invalid runner: read the first line (must start with `//`), take the expected
  error text, parse, assert failure and normalized message match.
- Classification file (`testdata/loader/CLASSIFICATION.md`) listing the valid
  2.1 fixtures and the invalid fixtures that are semantic, not syntactic.
- Current counts: 100 valid parse; 180 invalid are syntax-level and match; 27
  invalid are semantic and deferred to M2. Do not assume all 207 are syntax
  errors.

---

## 6. Acceptance criteria

- [x] `go build ./...` and `go test ./...` pass.
- [x] `go vet ./...` clean.
- [x] `gosmithy parse` emits JSON for a file, with source locations.
- [x] All 100 valid fixtures parse (including the 18 that use 2.1 syntax).
- [x] All 180 syntax-level invalid fixtures produce the expected error
      (normalized).
- [x] 2.1 constructs in 2.0 files produce a clear version-gated error, and 2.1
      files parse instead of panicking.
- [x] Fuzz target runs for a short smoke period without crashes.
- [x] `testdata/` attribution is present (`NOTICE` + `LICENSE`).
- [x] No dependency outside the Go standard library.

**Demo command**

```sh
go test ./idl/... ./cmd/...
go run ./cmd/gosmithy parse testdata/loader/valid/main-test.smithy
```

---

## 7. Risks

| Risk | Mitigation |
|---|---|
| Whitespace-sensitive grammar bugs | Port `BR`/`WS` semantics literally; test with the invalid corpus. |
| Text-block indentation/escapes | Dedicated unit tests from `valid/text-blocks.smithy`; fuzz strings. |
| Doc-comment capture | Test `valid/doc-comments/` explicitly; keep doc capture in the tokenizer, not parser hacks. |
| Invalid fixtures mix syntax and semantics | Classify at kickoff; do not chase semantic errors in M1. |
| 2.1 constructs leak into 2.0 fixtures | Version gate in tokenizer/parser; skip list is explicit, never silent. |
| Scope creep into semantic checks | Parser only resolves nothing; all shape references remain strings. |

---

## 8. Deliverables

- A Go module that parses the Smithy 2.0 IDL corpus and the 2.1 syntax
  additions.
- `gosmithy parse` CLI.
- Fixture harness and classification.
- Documented AST types and tokenizer behavior.
- The foundation for Milestone 2.
