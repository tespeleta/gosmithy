# gosmithy

`gosmithy` is a small Go implementation of the Smithy 2.0 interface definition
language, its semantic model, and a set of generators for Go code, OpenAPI, and
documentation.

It is for projects that want to read Smithy models and turn them into plain Go
code without pulling in a large toolchain. Feed it a `.smithy` file or a JSON
AST model, and it will resolve the model, report structural problems, and emit
a self-contained Go module containing data types, validators, and an optional
HTTP+JSON client and server. The same model can produce an OpenAPI document and
a generated documentation bundle.

This is **not** an attempt to reimplement Smithy or its surrounding ecosystem.
The scope is intentionally narrow. The project focuses on the low-level, stable
parts of the specification: the IDL, the semantic model, and the traits that
directly change generated output. No Java toolchain or JVM is involved.

The full map of what is and is not covered is in [`COVERAGE.md`](COVERAGE.md),
organised as a checkbox tree that follows the
[Smithy 2.0 specification](https://smithy.io/2.0/spec/index.html).

## Objectives

gosmithy aims to:

- **Read Smithy IDL 2.0** and the 2.1 language additions: inline collections,
  member indexes, and tagged literals.
- **Build a semantic model**: namespaces and `use`, the prelude, `apply`,
  forward references, mixins, inline operation I/O, duplicate merging, service
  closures, and 2.1 synthetic shapes.
- **Validate the structural rules code generation depends on**: unresolved
  targets, invalid members, operation I/O and errors, mixin problems, enum
  problems, and syntactic shape IDs.
- **Generate Go code** for structures, collections, enums, intEnums, unions,
  and errors, with `Validate()` methods for the core constraint traits.
- **Generate both sides of a simple HTTP+JSON service**: a typed client, a
  typed server, routing, error mapping, and JSON codecs.
- **Generate an OpenAPI 3.0.3 document** from the same service and HTTP
  bindings, for use with the wider OpenAPI tool ecosystem.
- **Generate API documentation**: a static Redoc API reference plus a
  Markdown guide derived from the model's documentation traits and examples.
- **Keep generated Go modules self-contained**: only the Go standard library,
  no runtime dependency on gosmithy.
- **Stay usable as a library**: `idl`, `model`, `validation`, and `codegen` are
  ordinary public Go packages.

What it deliberately does not try to do is equally important:

- It is not a drop-in replacement for the complete Smithy toolchain.
- It does not implement AWS protocols, authentication, signing, endpoints, or
  AWS-specific traits.
- It does not implement the full Smithy validator framework, selector engine,
  suppressions, severity overrides, model transformations, or projections.
- It does not read `smithy-build.json` or resolve Maven/Gradle/plugin setups.
- It does not generate code for languages other than Go.
- It does not implement streaming, event streams, XML, CBOR, multipart,
  compression, retries, or content negotiation.

Those are real Smithy features. They are simply outside this project's scope.

## Quick start

```sh
git clone https://github.com/tespeleta/gosmithy
cd gosmithy

make build
./bin/gosmithy version

# Inspect a model
./bin/gosmithy parse testdata/weather.smithy
./bin/gosmithy validate testdata/weather.smithy
./bin/gosmithy model testdata/weather.smithy

# Generate Go data types and validators
./bin/gosmithy generate \
    --service example.weather#Weather \
    --generator go:model \
    --goMod \
    --module example.com/weather \
    --out ./gen \
    testdata/weather.smithy

(cd gen && go build ./...)

# Generate an HTTP+JSON client and server
./bin/gosmithy generate \
    --service example.library#Library \
    --generator go:model,go:server,go:client \
    --goMod \
    --module example.com/library \
    --out ./gen-http \
    testdata/library.smithy

(cd gen-http && go build ./...)

# Generate Go, OpenAPI, and docs in one run
./bin/gosmithy generate \
    --service example.library#Library \
    --generator go:model,go:server,go:client,openapi:descriptor,openapi:index,docs:markdown \
    --goMod \
    --module example.com/library \
    --out ./gen-all \
    testdata/library.smithy
```

`make build` writes `bin/gosmithy` with `-trimpath` and embeds a version from
`git describe` (falling back to `0.1.0-dev`). You can also use
`go build ./cmd/gosmithy`, or `go run ./cmd/gosmithy ...` for a one-off run.

The generated modules contain no tests of their own. The repository's
round-trip, golden, and compile tests live under
[`codegen/golang`](codegen/golang), and the quick start checks generated code
with `go build ./...`.

## How it fits together

```text
Smithy IDL (.smithy) ─┐
                      ├─→ idl ─→ model ─→ validation
Smithy JSON AST (.json)┘          │
                                  └─→ codegen ─→ Go module
                                        │      └→ OpenAPI 3.0.3
                                        │      └→ docs (Redoc HTML + Markdown)
                                        └─ service closure + HTTP binding index
```

- [`idl`](idl) turns text or JSON into a source-located AST.
- [`model`](model) resolves names, assembles the prelude, flattens mixins,
  applies traits, and freezes the semantic model.
- [`validation`](validation) adds structural checks and returns source-located
  events.
- [`model/httpbinding`](model/httpbinding) builds the HTTP binding index for a
  service.
- [`codegen`](codegen) defines the generator contract and writes merged output.
- [`codegen/golang`](codegen/golang) is the Go target.

The dependency direction is `idl → model → validation → codegen → cmd`.

## Parsing

The parser produces a close-to-syntax AST with line/column locations on shapes,
members, traits, and nodes.

```go
import "github.com/tespeleta/gosmithy/idl"

file, err := idl.Parse("weather.smithy", src)
if err != nil {
    // Errors are *idl.ParseError and carry line/column locations.
    return err
}

for _, stmt := range file.Shapes {
    shape := stmt.(*idl.ShapeStmt)
    fmt.Println(shape.Kind, shape.Name)
}
```

Use `ParseWithOptions` to pin the supported and default IDL versions:

```go
file, err := idl.ParseWithOptions("weather.smithy", src, idl.Options{
    Version:        idl.V2_0, // newest version the caller supports
    DefaultVersion: idl.V2_0, // used when the file has no $version
})
```

`Version` gates 2.1 syntax. `DefaultVersion` is assumed when a file has no
`$version` control statement. The CLI supports 2.1 and assumes 2.0 when a file
omits `$version`.

JSON AST input is supported through `idl.ParseJSONAST`. The CLI selects it by
file extension, so `parse`, `validate`, `model`, and `generate` all accept
`.json` models. Loading JSON AST is supported; writing JSON AST is not.
`gosmithy parse` prints a debug JSON view of the AST, not a Smithy JSON AST.

## The model API

```go
import (
    "github.com/tespeleta/gosmithy/idl"
    "github.com/tespeleta/gosmithy/model"
    "github.com/tespeleta/gosmithy/validation"
)

file, err := idl.Parse("weather.smithy", src)
if err != nil {
    return err
}

m, err := model.Assemble([]*idl.File{file}, model.Options{})
if err != nil {
    return err // internal prelude failure, not a modeler error
}

for _, event := range validation.Validate(m) {
    fmt.Println(event) // sorted by source location
}

if closure, ok := m.ServiceClosure(model.MustShapeID("example.weather#Weather")); ok {
    _ = closure
}
```

Assembly never fails on an invalid user model. Problems are recorded as
diagnostics and returned by `validation.Validate`. Member shapes are first-class
entries in the model (`Foo$bar`), so member traits and traversal are uniform with
other shapes. Shapes, members, traits, and metadata are iterated
deterministically, and the model is frozen for code generation.

`model.Node` is a resolved node value. Syntactic shape IDs (unquoted identifiers
in node values) are resolved to absolute shape IDs and marked with
`Node.SyntacticShapeID`; validation reports unresolvable ones with the
`SyntacticShapeIdTarget` event.

## Code generation

A generator receives one request and returns files. The Go target and the
OpenAPI/docs generators use the same contract. Generators never write to disk,
which makes them deterministic and easy to test:

```go
type Generator interface {
    Generate(ctx context.Context, req *Request) (*Response, error)
}

type Request struct {
    Model    *model.Model
    Service  model.ShapeID
    Version  model.Version
    Target   Target
    Protocol *Protocol          // nil for model-only generation
    Options  map[string]string
    Files    []string
}
```

`codegen.Runner` calls every selected generator with the same request, merges
their files, rejects paths outside the output directory, detects duplicate paths
with differing content, and writes only after every generator succeeds. DANGER
diagnostics abort the run.

The output directory is owned by the tool. Pass `--clean` to remove it before
writing, so outputs from an older model do not linger. Without `--clean` the
runner only writes and overwrites files; files it no longer generates stay
behind.

A model-only Go generation produces:

```text
gen/
  go.mod            # only with --goMod
  internal/smithy/
    json.go
    validate.go
    relaxed.go     # when relaxed numeric members are generated
  types/
    types.go
    enums.go
    unions.go
    validate.go
    json.go        # when custom codecs or required scalar presence are needed
    decimal.go     # when the model uses bigDecimal
    relaxed.go     # when relaxed boolean members are generated
    optional.go    # when non-required scalar members use Optional
```

The generated module uses only the standard library. The Go target maps Smithy
shapes as follows:

| Smithy | Go |
|---|---|
| `string` | `string` |
| `blob` | `[]byte` |
| `boolean` | `bool` |
| `byte` / `short` / `integer` / `long` | `int8` / `int16` / `int32` / `int64` |
| `float` / `double` | `float32` / `float64` |
| `bigInteger` / `bigDecimal` | `*big.Int` / `*Decimal` |
| `timestamp` | `time.Time` |
| `document` | `any` |
| `list` / `set` | `[]T` |
| `map` | `map[string]V` |
| `structure` | `struct` |
| `union` | interface plus per-member wrapper structs |
| `enum` | named `string` plus constants |
| `intEnum` | named `int32` plus constants |
| `Unit` | `struct{}` |
| error structure | struct implementing `error` + `ErrorCode()` |

### Optional Go members

By default (`--useOptionals=true`), non-`@required` scalar members are
`Optional[T]`, and `@required` scalar members are plain `T`:

```go
type In struct {
    Name  Optional[string]    `json:"name,omitzero"`
    Count Optional[int32]     `json:"count,omitzero"`
    Ok    Optional[bool]      `json:"ok,omitzero"`
    Id    string              `json:"id"` // @required
}

in.Name.Set("x")
if in.Count.HasValue() {
    use(in.Count.MustValue())
}
```

`Optional` distinguishes absent, explicit `null`, and a value; `omitzero`
omits absent fields. It needs Go 1.24, so a generated `go.mod` declares
`go 1.24`. Pass `--useOptionals=false` to restore the pointer representation
and `go 1.22`. HTTP-bound and `@httpPayload` members keep pointers regardless
of this setting. `bigDecimal` uses `Optional[Decimal]`, and `bigInteger` uses
`Optional[*big.Int]`.

Members use `Optional[T]` for non-required scalars, pointers for structures and
HTTP-bound values, and nilable slices/maps/blobs. Requiredness and constraints
are enforced by `Validate()`. Required scalars stored as plain Go values get
their presence from the generated `UnmarshalJSON`, which records members that
were absent or `null`; `Validate()` reports those with an `is required` error.
A value built directly in Go carries no presence information, so only decoded
values are checked this way. Structures, errors, and named collections all get
a `Validate()` method covering `@required`, `@length`, `@range`, `@pattern`,
`@uniqueItems`, enum and intEnum membership, and nested shapes. Member
constraints override the same constraint on the target shape. Failure paths
look like `nested.field`, `items[0]`, or `index[key]`.

`--list-generators` prints the namespaces and options:

```text
docs: markdown
go: client, model, server
openapi: descriptor, index
```

Repeatable `--opt key=value` flags pass generator-specific options. Go
generation does not write a `go.mod` by default, since it usually targets an
existing module; pass `--goMod` (or `--opt goMod=true`) to include one.

## HTTP+JSON clients and servers

The Go namespace has three options; the bare `go` namespace selects all three:

| Generator | Emits |
|---|---|
| `go:model` | The base module: types, validators, and codecs. |
| `go:server` | The base module plus the service interface, server, protocol helpers, and HTTP runtime. |
| `go:client` | The base module plus the client, protocol helpers, and HTTP runtime. |
| `go` | All Go options: model, server, and client. |

Select `go:server,go:client` (or simply `go`) for a full client and server:

```text
gen-http/
  go.mod            # only with --goMod
  service.go
  client.go
  server.go
  protocol.go
  types/
  internal/smithy/
```

`gosmithy.protocols#httpJson` is the project's own generic protocol. It is not
an AWS protocol and does not implement AWS behavior.

Operations use `@http` plus the standard HTTP traits:

| Trait | Behavior |
|---|---|
| `@http(method, uri, code)` | Required on every operation. `code` defaults to 200. |
| `@httpLabel` | One required input member per `{label}` URI segment. Percent-encoded on the client and decoded on the server. |
| `@httpQuery` | Query parameter. Lists become repeated keys; scalars use their string form. |
| `@httpQueryParams` | Map of unbound query parameters. Keys bound with `@httpQuery` are excluded. |
| `@httpHeader` | Request or response header. Lists are comma-separated. |
| `@httpPrefixHeaders` | Map of prefixed request or response headers. |
| `@httpPayload` | Raw request or response body. Blob is raw bytes; string is text; other shapes are JSON. |
| `@httpResponseCode` | Output member that overrides the response status. |
| `@httpError` | Status code for a modeled error. Defaults to 400 for client errors and 500 for server errors. |

Unbound members are serialized into the JSON body. Bound fields are removed
before marshaling and are overwritten from the HTTP message after unmarshaling.
A payload may not be combined with unbound body members. Unknown JSON fields are
ignored when decoding structures, for forward compatibility.

### Relaxed input

The optional `gosmithy.protocols#relaxedInput` trait makes JSON input decoding
accept quoted scalar values. It can be applied to a service or to a scalar
member; a member setting wins over the service setting.

```smithy
use gosmithy.protocols#relaxedInput

@relaxedInput
service S { ... }

structure In {
    // Strict for this service.
    @relaxedInput(enabled: false)
    count: Integer

    // Relaxed for this service.
    ok: Boolean
}
```

| Smithy target | Strict input | Relaxed input |
|---|---|---|
| `boolean` | `true`, `false` | also `"true"`, `"false"` |
| `byte`...`double` | JSON number | also a quoted JSON number |
| `bigInteger` | JSON integer | also a quoted integer |
| `bigDecimal` | JSON number | also a quoted number |

Relaxed input applies only to operation inputs and only to direct scalar
members; collections, maps, and aggregate members are not relaxed in v1.
HTTP label, query, header, and response-code bindings are always strings and
are not affected. Encoding is always strict: generated code writes bare JSON
booleans and numbers.

`bigDecimal` maps to `types.Decimal`, which stores the JSON number literal and
preserves its digits (including trailing zeros and exponents). Construct one
with `types.NewDecimal`/`types.MustDecimal`, or `DecimalFromInt64` and
`DecimalFromBigInt`. Strict `bigDecimal` input accepts a JSON number only; a
quoted decimal is accepted only under `@relaxedInput`.

The generated API is ordinary Go:

```go
type Service interface {
    GetBook(ctx context.Context, input *types.GetBookInput) (*types.GetBookOutput, error)
}

func NewServer(service Service, opts ...ServerOption) *Server
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)

func NewClient(endpoint string, opts ...ClientOption) *Client
func (c *Client) GetBook(ctx context.Context, input *types.GetBookInput, optFns ...func(*Options)) (*types.GetBookOutput, error)
```

Client options are `WithHTTPClient`, `WithBasePath`, and `WithHeader`. Per-call
`optFns` receive a copied `Options` value, so headers and base paths do not leak
between calls. Server options are `WithMaxBodyBytes` (default 10 MiB),
`WithErrorLogger`, and `WithServerBasePath`. The server is a plain
`http.Handler`, so ordinary middleware works.

Modeled errors are returned as their generated Go types. Other error responses
become `*smithy.ProtocolError` with `Code`, `Message`, `StatusCode`, and the raw
body.

Errors use a small JSON envelope:

```json
{
  "code": "NotFound",
  "message": "resource not found",
  "resource": "book"
}
```

| Situation | Status | Code |
|---|---:|---|
| Unknown route | 404 | `NotFound` |
| Wrong method | 405 plus `Allow` | `MethodNotAllowed` |
| Malformed request | 400 | `BadRequest` |
| Input validation failure | 400 | `ValidationException` (with `errors`) |
| Body limit exceeded | 413 | `PayloadTooLarge` |
| Unhandled service error or panic | 500 | `InternalFailure` |

`testdata/http.smithy` exercises every binding and error path. The Go generator
tests generate it into a temporary module and run an `httptest` round trip for
labels, query parameters, headers, prefix headers, JSON bodies, blob payloads,
unions, big numbers, timestamps, modeled errors, unknown errors, validation
failures, and empty operations.

## OpenAPI and documentation

Two namespaces consume the same assembled model and HTTP binding index:

- **`openapi:descriptor`** emits the root `openapi.json`, an OpenAPI 3.0.3
  document with paths, parameters, request/response bodies, modeled errors,
  component schemas, constraints, and documentation traits.
- **`openapi:index`** emits `docs/index.html` (Redoc) and a self-contained
  `docs/openapi.json` for the viewer.
- **`docs:markdown`** emits `docs/README.md`, the generated overview,
  operations, schemas, and examples.

The bare namespaces `openapi` and `docs` select all of their options.

Generate one option, a namespace, or several namespaces:

```sh
# Descriptor only. No Go module is generated, so --module is not required.
./bin/gosmithy generate \
    --service example.library#Library \
    --generator openapi:descriptor \
    --out ./api \
    testdata/library.smithy

# The full documentation bundle: Redoc HTML, docs-local descriptor, and Markdown.
./bin/gosmithy generate \
    --service example.library#Library \
    --generator openapi,docs \
    --out ./site \
    testdata/library.smithy

# Go, OpenAPI, and docs in one run.
./bin/gosmithy generate \
    --service example.library#Library \
    --generator go:model,go:server,go:client,openapi:descriptor,openapi:index,docs:markdown \
    --module example.com/library \
    --out ./gen \
    testdata/library.smithy
```

`--generator` is repeatable, and each value may also be a comma-separated list.
The runner merges the outputs and reports the total number of files written.
The wire protocol is always `gosmithy.protocols#httpJson`, the only built-in
protocol; there is no protocol flag. `--useOptionals=false` switches generated
Go scalar members back from `Optional[T]` to pointers and keeps the generated
module on Go 1.22.

The generators cover the documentation traits that affect API documentation:

| Trait | OpenAPI | Generated Markdown |
|---|---|---|
| `@documentation`, `///` | `description` | prose |
| `@deprecated` | `deprecated` | badge |
| `@title` | `info.title`, operation summary, schema title | headings |
| `@tags` | tags and operation grouping | tags |
| `@externalDocumentation` | `externalDocs` (service/operations), `x-smithy-external-docs` (schemas) | external links |
| `@examples` | parameter, request, and response examples | example blocks |
| `@sensitive` | `format: password` | password type/badge |
| `@since` | `x-smithy-since` | `Since …` badge |
| `@recommended` | `x-smithy-recommended` | `Recommended` badge |
| `@unstable` | `x-smithy-unstable` + feature id | `Unstable (feature …)` badge |
| `@unstableFeatures` | `x-smithy-unstable-features` | service-level note |
| `@internal` | `x-smithy-internal` | `Internal` badge |
| `@synthetic` | `x-smithy-synthetic` | “Generated by the assembler” badge |

Enum value documentation, deprecation, and `@since` values are carried in
`x-enum-descriptions`, `x-enum-deprecated`, and `x-enum-since` extensions.

`@internal` follows upstream Smithy behavior: internal operations and shapes
are included and marked, not filtered. Filtering can be added as an opt-in
later.

The OpenAPI generator warns and omits bindings that OpenAPI cannot represent,
notably `@httpPrefixHeaders` and dynamic `@httpResponseCode`. The static
`@http(code)` is used for the success response. The generated `index.html`
loads Redoc from a CDN and reads `openapi.json` at runtime, so serve the docs
over HTTP rather than opening the file directly:

```sh
python3 -m http.server --directory ./gen/docs
```

All other generated docs are plain Markdown and JSON. `testdata/docs.smithy`
exercises the documentation traits, examples, synthetic collections, and
generated docs end to end.

## CLI

```text
gosmithy <command> [arguments]

Commands:
  parse <file>...      Parse Smithy IDL or JSON AST and print the AST as JSON
  validate <file>...   Assemble and validate a Smithy model
  model <file>...      Assemble a model and print a summary or debug JSON
  generate <file>...   Generate code, OpenAPI, or docs for a service
  version              Print the version
  help                 Print help
```

Useful flags:

| Command | Flags |
|---|---|
| `validate` | `-min-severity note\|warning\|danger`, `-json` |
| `model` | `-json`, `-prelude=false`, `-protocols=false` |
| `generate` | `--service`, `--generator` (namespace or `namespace:option`; repeatable/comma-separated; default all), `--module` (required for Go), `--goMod` (write `go.mod`; default off), `--out`, `--clean`, `--list-generators`, `--opt key=value` |

`--generator go` selects all Go options; `--generator go:model` selects only
the base module. Omitting `--generator` selects every namespace and option.
There is no protocol flag.

`validate` exits 1 when any DANGER event is reported. Events are printed as
`file:line:column: SEVERITY: message`. `validate -json` emits the same events as
structured JSON. Normal output goes to stdout; fatal errors and usage go to
stderr.

`model --json` prints a deterministic debug view of the assembled semantic
model. It is not a Smithy JSON AST. Pass `-prelude=false` to omit prelude shapes
and `-protocols=false` to omit the built-in wire protocol definitions. The
`smithy.protocols#idx` trait used by 2.1 member indexes is always built in.

`validate`, `model`, and `generate` assemble the embedded
`gosmithy.protocols#httpJson` definition by default, so protocol models need no
extra flag. `model -protocols=false` omits it. `generate` always targets
`gosmithy.protocols#httpJson`; there is no protocol-selection flag.

## Packages

```text
cmd/gosmithy/    CLI: parse, validate, model, generate, version, help
codegen/         generator contract, registry, runner, conflict detection
codegen/golang/  Go target: naming, type mapping, types, validators, HTTP
codegen/openapi/ OpenAPI 3.0.3 generator
codegen/docs/    Markdown documentation generator
internal/runtime/ source for the generated internal/smithy package
idl/             tokenizer, AST, parser, JSON AST loader, fixture tests
model/           semantic model, prelude, assembly, mixins, service closure
model/httpbinding/ HTTP binding index
protocol/httpjson/ embedded gosmithy.protocols#httpJson definition
validation/      structural validators and events
testdata/        fixture corpus, examples, and golden codegen output
```

## Development

```sh
make check   # gofmt check + go vet + go test
```

| Target | Action |
|---|---|
| `make build` | Build `bin/gosmithy` (default) |
| `make test` | Run `go test ./...` |
| `make test-race` | Run tests with the race detector |
| `make vet` | Run `go vet ./...` |
| `make fmt` | Format all Go sources |
| `make fmt-check` | Fail if any source is not gofmt-clean |
| `make check` | `fmt-check` + `vet` + `test` |
| `make fuzz` | Run `FuzzParse` for `FUZZTIME` (default 10s) |
| `make run` | Build and run the CLI (`ARGS=...`) |
| `make install` | Install into `GOBIN`/`GOPATH/bin` |
| `make clean` | Remove `bin/` |

The parser and model suites run the upstream Smithy fixture corpus. Every valid
fixture must parse and assemble; syntax-invalid fixtures must produce their
expected parser error; semantic-invalid fixtures are checked by event ID,
message, and source location. `testdata/loader/CLASSIFICATION.md` explains the
split between syntax and semantic failures.

Mixin flattening is compared against the upstream `.flattened.smithy` fixtures.
The JSON AST loader is compared with IDL siblings for every upstream JSON
fixture that has one.

The `codegen/golang` tests generate the weather, validator, library, and HTTP
fixtures, compare golden output, compile and vet the generated modules, and run
generated validator behavior tests in a temporary module. The
`codegen/openapi` and `codegen/docs` tests compare OpenAPI JSON and Markdown
golden files and cover documentation traits, examples, and multi-generator CLI
runs. Update golden output intentionally with:

```sh
go test ./codegen/golang -run TestGolden -update
go test ./codegen/openapi -update-openapi
go test ./codegen/docs -update-docs
```

A short fuzz smoke run:

```sh
make fuzz                  # 10s by default
make fuzz FUZZTIME=30s
```

## Scope and limitations

The detailed, section-by-section view is in [`COVERAGE.md`](COVERAGE.md). The
short version:

- **No AWS.** No AWS protocols, auth, signing, endpoints, SigV4, or AWS traits.
- **No full validator framework.** No selector engine, complete built-in
  validator suite, suppressions, severity overrides, custom validators, or model
  transforms.
- **No build/project system.** No `smithy-build.json`, projections, or
  Maven/Gradle/plugin resolution.
- **No JSON AST output.** Loading is supported; `model --json` is a debug view,
  not a Smithy JSON AST serializer.
- **One code target.** Go is the only language target. OpenAPI and docs are
  additional output formats, not language backends.
- **One OpenAPI shape.** OpenAPI 3.0.3 JSON only; no YAML or 3.1 output yet.
  No `servers`, security schemes, AWS extensions, or OpenAPI features that
  Smithy cannot express.
- **Docs are API docs.** The generated Markdown covers the service, operations,
  schemas, and examples. Hand-written guides and tutorials remain hand-written.
  Redoc is loaded from a CDN; the HTML is not a pre-rendered static site.
- **One wire protocol.** `gosmithy.protocols#httpJson` is the only protocol. It
  has no AWS compatibility, signing, authentication, retries, streaming, event
  streams, compression, multipart, XML, CBOR, or content negotiation.
- **Limited trait semantics.** Many traits are parsed and stored but have no
  effect. Resource binding rules, `@idRef` selectors, `@indexed` validation,
  endpoint traits, authentication traits, and most behavior traits are not
  modeled beyond their data.
- **Go model limits.** `@default` and `@clientOptional` are not applied; union
  values and map keys are not validated; `@pattern` uses Go's RE2 syntax;
  validators do not detect cycles; client input and server output are not
  validated.
- **HTTP binding limits.** Literal query strings in URIs and greedy labels are
  rejected; ambiguous routes are rejected instead of resolved by specificity;
  restricted headers are not diagnosed; error structures do not support
  `@httpHeader` or `@httpPayload`; header timestamps default to `date-time`
  instead of `http-date`.

`COVERAGE.md` records the exact boundary, including the parts that are partial.

## License

Apache-2.0. See [`LICENSE`](LICENSE).

## Attribution

The fixture corpus under `testdata/loader/` is derived from the upstream Smithy
repository and is licensed under Apache-2.0. See
[`testdata/NOTICE`](testdata/NOTICE) and [`testdata/LICENSE`](testdata/LICENSE).

The embedded prelude in [`model/prelude.smithy`](model/prelude.smithy) is based
on the upstream prelude, with one gosmithy extension (`epoch-milliseconds` in
`timestampFormat`), and is licensed under Apache-2.0.
