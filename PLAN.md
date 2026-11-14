# gosmithy — plan

Module: `github.com/tespeleta/gosmithy`
Status: proposal v2 · 2026-11-14 · Milestones 1–4 implemented

A Go implementation of the Smithy IDL and a pluggable code generator. It reads
Smithy models and generates Go types, validators, clients, and servers.

The core is generic Smithy. **No AWS-specific protocols, traits, or wire
formats are part of the MVP.**

---

## 1. Scope

### 1.1 What we build

```
Smithy IDL text
  → tokens
  → AST
  → semantic model (resolved shapes + traits + prelude)
  → validation
  → indexes (service closure, HTTP bindings)
  → code generators (pluggable)
```

### 1.2 What we do not build (for now)

- AWS protocols (`awsJson1_1`, `restJson1`, `awsQuery`, ...), AWS auth,
  endpoints, SigV4.
- The full ~78-validator suite, selector engine, custom validators,
  suppressions.
- Semantic assembly of 2.1-only syntax: synthetic inline-collection shapes,
  synthetic naming, and `idx` application are M2 work. M1 parses the syntax.
- `smithy-build.json`, projections, transforms, plugin/Maven resolution.
- JSON AST loading (a debug model JSON view is not a conformance claim; full loader later).
- Streaming, event streams, XML, CBOR, auth, retries.
- A generic multi-language codegen framework ported from Java.

Those are real Smithy features, but none are needed to prove the core.

### 1.3 Prior art

- The official Go runtime is [`aws/smithy-go`](https://github.com/aws/smithy-go).
  The official Go code generators are Java, WIP, and do not provide a general
  server protocol implementation.
- [`boynton/smithy`](https://github.com/boynton/smithy) is a Go Smithy 1.0
  implementation. Useful parser prior art, not a 2.0 base.
- Codegen architecture takes inspiration from Buf: small generator plugins,
  a request/response contract, no generator writing files directly, and a
  runner that composes outputs.

---

## 2. Principles

1. **Core is generic Smithy.** AWS-specific behavior belongs in optional
   plugins, never in the parser/model/validator.
2. **One direction of dependency.** `idl` → `model` → `validation` →
   `codegen` → `cmd`. No cycles; no codegen imports in the model.
3. **Generators are plugins.** A generator receives a request and returns files.
   It does not write to disk, read global config, or know the CLI.
4. **Three pluggable axes.**
   - **Spec version** — parser feature set; MVP targets Smithy 2.0.
   - **Codegen target** — language/backend; first target is Go.
   - **Protocol** — wire format for client/server; first is our generic
     `gosmithy.protocols#httpJson` (M4), not AWS.
5. **Self-contained generated code.** A generated module embeds a small
   `internal/smithy` runtime and depends only on the standard library.
6. **Fixtures are the specification.** Build against the Java repo's real IDL
   corpus (100 valid, 207 invalid) from the first milestone.
7. **Small, boring increments.** Every milestone is independently shippable and
   has a concrete demo.

---

## 3. Architecture

### 3.1 Packages

```
github.com/tespeleta/gosmithy/
  go.mod
  cmd/gosmithy/               # CLI: parse, validate, model, generate
  idl/                        # tokenizer, parser, AST
  model/                      # semantic model, prelude, assembly, closure
  validation/                 # events + minimal checks
  codegen/                    # plugin contract + runner (no language logic)
  codegen/golang/             # Go target: naming, types, validators, http
  protocol/httpjson/          # generic protocol (M4)
  internal/runtime/           # source for the generated internal/smithy package
  testdata/                   # fixture corpus
  docs/plan/                  # one markdown per milestone
```

No `internal/` around `idl`, `model`, `codegen`: these are the public building
blocks. Use `internal/` only for generated-runtime templates and test helpers.

### 3.2 Core model types (sketch)

```go
type ShapeID struct {
    Namespace string
    Name      string
    Member    string
}

type Shape struct {
    ID        ShapeID
    Type      ShapeType
    Traits    TraitMap       // map[ShapeID]Trait
    Members   []*Shape       // member shapes are first-class
    Target    ShapeID        // member shapes
    Container ShapeID        // member shapes
    Source    SourceLocation

    // type-specific properties, flat for MVP
    Input, Output ShapeID
    Errors        []ShapeID
    Mixins        []ShapeID
    Service       *ServiceProps
    Resource      *ResourceProps
}

type Trait struct {
    ID     ShapeID
    Value  Node
    Source SourceLocation
}

type Node struct {
    Kind   NodeKind
    Str    string
    Num    json.Number
    Bool   bool
    Arr    []Node
    Obj    map[string]Node
    Source SourceLocation
    // syntactic shape IDs are marked and resolved during assembly
    SyntacticShapeID bool
}
```

- Member shapes are first-class, as in Smithy's semantic model. This makes
  `Foo$bar`, member traits, and traversal uniform.
- Traits stay generic. Typed accessors for ~15–20 traits used by validation and
  codegen. No 100 trait classes.
- `SourceLocation` on shapes, members, traits, and nodes. Error quality is a
  feature.

### 3.3 Codegen plugin contract (Buf-inspired)

```go
package codegen

// Generator is the only interface a codegen target implements.
type Generator interface {
    Generate(ctx context.Context, req *Request) (*Response, error)
}

type Request struct {
    Model    *model.Model
    Service  model.ShapeID
    Version  model.Version        // target spec version (2.0 first)
    Target   Target               // "go"
    Protocol *Protocol            // nil for model-only generation
    Options  map[string]string    // generator-specific options
    Files    []string             // source files, for diagnostics only
}

type Response struct {
    Files       []File
    Diagnostics []Diagnostic
}

type File struct {
    Path    string
    Content []byte
    Mode    fs.FileMode
}
```

- `codegen.Registry` maps a name (`"go"`) to a generator factory.
- `codegen.Runner` selects generators, calls them once with the same request,
  merges files, detects path conflicts, and writes the output tree.
- Generators return bytes, never write files. This makes golden tests trivial
  and prevents plugin I/O surprises.
- In-process plugins for MVP. A future `codegen/pluginproto` adapter can expose
  the same request/response over stdin/stdout (JSON) for external generators,
  the way Buf does with protoc plugins. The interface above is the stable core.
- Multiple generators can run in one invocation and write into one module;
  conflict detection is the runner's job.

A target is a bundle of generators, not a parallel abstraction:

```go
type Target struct {
    Name       string
    Generators []string // e.g. "go-types", "go-http"
}
```

For MVP the registry can contain one generator, `"go"`, that emits types and
validators. Splitting it into `go-types` / `go-http` later is an additive change.

### 3.4 Protocol contract (M4)

A protocol is a model trait plus a Go codegen/runtime implementation. The first
protocol is ours, not AWS:

```smithy
namespace gosmithy.protocols

@protocolDefinition(traits: [
    http
    httpError
    httpHeader
    httpLabel
    httpPayload
    httpPrefixHeaders
    httpQuery
    httpQueryParams
    httpResponseCode
    jsonName
    mediaType
    timestampFormat
])
@trait(selector: "service")
structure httpJson {}
```

Semantics, documented fully in Milestone 4:

- `@http(method, uri, code)` is required per operation.
- `@httpLabel`, `@httpQuery`, `@httpHeader`, `@httpPrefixHeaders`,
  `@httpPayload`, `@httpResponseCode` follow the Smithy HTTP binding spec.
- Unbound input/output members are JSON bodies.
- Errors are JSON objects with `code`, `message`, and error members; status from
  `@httpError`, default 400 (client) / 500 (server).
- Timestamps default to RFC 3339; blobs are base64; documents are raw JSON.
- No signing, auth, retries, or streaming.

This gives generic HTTP+JSON without importing any AWS convention. AWS protocols
can later be separate optional plugins if someone needs them.

### 3.5 Version target

```go
type Version struct{ Major, Minor int }

var (
    V2_0 = Version{2, 0} // MVP
    V2_1 = Version{2, 1} // syntax supported in M1; semantics in M2
)
```

- M1 parses IDL 2.0 and the 2.1 syntax additions: inline collection targets,
  member index shorthand, and tagged string literals.
- `$version: "2"` resolves to the newest minor version the parser supports
  (2.1), as required by the spec. `$version: "2.1"` requests 2.1 explicitly.
- 2.1 constructs in a file that declares 2.0 (or defaults to it) produce a
  clear "requires Smithy IDL version 2.1" error.
- 2.1 semantics are deliberately deferred: M1 records inline targets and
  member indexes in the AST; M2 creates synthetic collection shapes and
  applies `smithy.protocols#idx`.
- The parser takes `idl.Options{Version, DefaultVersion}`. `Version` caps the
  supported feature set, and `DefaultVersion` is assumed when a file omits
  `$version` (2.0 by default, per the milestone-1 plan).

### 3.6 Core simplifications

1. **Generic traits + typed accessors**, not 100 trait classes.
2. **Multi-phase assembly** (parse files, then resolve/apply/merge), not
   Java's load-operation callbacks. Forward references are normal.
3. **Go-specific emitter** inside the plugin, not the Java codegen framework.
4. **Minimal validator set**, not 78 validators.
5. **Self-contained generated runtime**, not `smithy-go`.
6. **One generic protocol**, not AWS compatibility in the core.

### 3.7 Go type mapping (MVP)

| Smithy | Go |
|---|---|
| `string` | `string` |
| `blob` | `[]byte` |
| `boolean` | `bool` |
| `byte`/`short`/`integer`/`long` | `int8`/`int16`/`int32`/`int64` |
| `float`/`double` | `float32`/`float64` |
| `bigInteger`/`bigDecimal` | `*big.Int`/`*big.Float` |
| `timestamp` | `time.Time` |
| `document` | `any` |
| `list`/`set` | `[]T` |
| `map` | `map[string]V` |
| `structure` | `struct` |
| `union` | interface + per-member wrapper struct |
| `enum` | named `string` + constants |
| `intEnum` | named `int32` + constants |
| `Unit` | `struct{}` |
| error structure | struct implementing `error` + `ErrorCode()` |

Nullability is intentionally simple at first: scalar/enum/structure/union members
are pointers, collections/blobs/documents are nilable, and requiredness is
enforced by `Validate()` (nil = absent). Refine against `@default` /
`@clientOptional` after the first vertical slice.

---

## 4. Milestones

Each milestone is **one unit of work** and one markdown document. A milestone is
only split when the two halves are independently shippable; the project skeleton
and the parser are not, so they are one milestone.

| # | Milestone | Unit of work | Deliverable | Depends on | Estimate |
|---|---|---|---|---|---|
| 1 | [Parser foundation](docs/plan/01-parser-foundation.md) | Skeleton + fixture harness + tokenizer/parser/AST | `gosmithy parse` passes the 2.0 IDL corpus | — | 6d |
| 2 | [Semantic model](docs/plan/02-semantic-model.md) | Resolution + prelude + structural validation | `gosmithy validate` / `model` on real models | 1 | 7d |
| 3 | [Codegen core + Go models](docs/plan/03-codegen-core-and-go.md) | Plugin contract + first Go generator | Generated Go module with types + validators, compiles | 2 | 6d |
| 4 | [Generic HTTP+JSON](docs/plan/04-http-json-protocol.md) | Protocol + HTTP bindings + client/server | Round-trip generated client/server for `httpJson` | 3 | 9d |

- **MVP boundary: end of Milestone 3.** Parsing, validation, Go models, and
  validators are already a real tool.
- **Full original goal: end of Milestone 4 (implemented).** Generated server and
  client, using our own generic protocol.
- Milestones 1–3 are the critical path; Milestone 4 can be deferred without
  blocking the parser/model/codegen foundation.

### Milestone summaries

**M1 — Parser foundation (skeleton + parser).**
Repository, module, CLI shell, fixture corpus, tokenizer, AST, parser for IDL
2.0 and 2.1 syntax (member indexes, inline collections, tagged literals),
source locations/doc comments, parser tests and fuzz smoke. No semantic
resolution. Exit: every valid fixture parses; every syntax-level invalid
fixture produces the expected error; semantic invalid fixtures are classified
for M2.

**M2 — Semantic model.**
Nodes, shape IDs, shapes/members, traits, prelude, multi-file assembly, relative
IDs, forward references, mixins, inline I/O, merge/conflict rules, service
closure, minimal structural validation. Exit: prelude + weather + 2.0 fixtures
assemble; `gosmithy validate` reports usable events.

**M3 — Codegen core + Go models/validators. Implemented.**
`codegen.Generator` contract, registry, runner, CLI `generate`, Go naming/type
mapping/writer, structs/enums/intEnums/unions/errors, `Validate()` methods,
embedded `internal/smithy` runtime, golden and compile tests. Exit: generated
module compiles and validates sample data.

**M4 — Generic HTTP+JSON. Implemented.**
`gosmithy.protocols#httpJson` trait, HTTP binding index, request/response
codec, generated client and server, routing, errors, validation → 400,
round-trip tests over `httptest`. Exit: a realistic service generates, compiles,
and round-trips. No AWS.

---

## 5. Testing strategy

1. **Parser corpus.** Use the Java repo fixtures:
   - `testdata/loader/valid/**` — must parse.
   - `testdata/loader/invalid/**` — first line is the expected error; classify
     syntax vs semantic, and only assert syntax in M1.
   - valid 2.1 fixtures parse; 2.1 constructs in 2.0 files are tested as
     version-gated errors.
2. **Model tests.** Prelude, weather example, forward refs, `use` aliases,
   apply-to-member, mixins, inline I/O, merge rules, closure.
3. **Validation tests.** Specific invalid models with expected event IDs and
   source locations.
4. **Codegen golden tests.** Generate into `testdata/golden/<case>`; compare
   with `-update`; every golden output must `go build`.
5. **End-to-end generated tests.** Write a temp module, run `go test ./...`,
   start `httptest.Server` with the generated handler, call the generated
   client, assert decoded values and errors.
6. **Fuzzing.** Tokenizer/parser fuzz target seeded from fixtures.

No differential testing against the Java CLI in MVP. The fixtures already encode
the expected behavior; add differential testing later if drift becomes an issue.

---

## 6. Risks and open decisions

| Risk | Mitigation |
|---|---|
| Parser edge cases (whitespace sensitivity, text blocks, docs, error recovery) | Build against fixtures from day one; fuzz. |
| Semantic resolution (forward refs, use, mixins, inline I/O, merge) | Multi-phase assembly; one focused test per mechanism; prelude as dogfood. |
| Protocol scope creep | MVP has no protocol; `httpJson` is deliberately small; AWS stays out. |
| Plugin architecture over-engineering | One interface, one runner, in-process only. External plugins later. |
| Nullability/default semantics | Simple documented rules; isolate in one file; revisit. |
| Fixture licensing | Apache-2.0 attribution in `testdata/NOTICE`. |

Decisions still open:

1. Go generator granularity: one `"go"` generator for MVP (recommended), or
   separate `"go-types"` / `"go-validators"` from the start?
2. CLI config: flags only for MVP (recommended), or a small config file?
3. Protocol naming: `gosmithy.protocols#httpJson` (recommended) — confirm.

---

## 7. Next action

Milestones 1–4 are implemented; see
[`docs/plan/01-parser-foundation.md`](docs/plan/01-parser-foundation.md),
[`docs/plan/02-semantic-model.md`](docs/plan/02-semantic-model.md),
[`docs/plan/03-codegen-core-and-go.md`](docs/plan/03-codegen-core-and-go.md), and
[`docs/plan/04-http-json-protocol.md`](docs/plan/04-http-json-protocol.md), plus
the protocol guide in [`docs/http-json-protocol.md`](docs/http-json-protocol.md).
The first post-M4 follow-up is the JSON AST loader described in
[`docs/plan/04-http-json-protocol.md`](docs/plan/04-http-json-protocol.md#10-deferred-to-the-end-json-ast-loader).
No AWS protocols, traits, or code paths are part of the project.
