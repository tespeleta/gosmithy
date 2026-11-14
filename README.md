# gosmithy

A Go implementation of the Smithy IDL and a pluggable code generator.

- Core is generic Smithy. No AWS protocols or traits in the MVP.
- Parse Smithy IDL 2.0 — plus the 2.1 syntax additions — into a source-located AST.
- Assemble a semantic model: prelude, namespaces, `use`, forward references,
  `apply`, mixins, inline operation I/O, duplicate merging, and service closures.
- Validate the structural properties codegen depends on.
- Generate Go types, validators, and HTTP+JSON clients and servers.
- Generators implement a small Buf-inspired request/response interface.
- Generated modules are self-contained and use only the standard library.

## Status

**Milestones 1–4 are implemented:** the parser covers the full IDL 2.0 grammar
plus the 2.1 syntax additions; the semantic model assembles the prelude,
fixtures, mixins, inline I/O, and service closures; structural validation
reports unresolved references, invalid members, invalid operation I/O and
errors, mixin problems, enum problems, and syntactic shape IDs; the `go`
code generator emits a self-contained Go module with data types and
validators; and `gosmithy.protocols#httpJson` adds an HTTP binding index,
generated clients and servers, JSON value codecs, and protocol errors. See
[`docs/http-json-protocol.md`](docs/http-json-protocol.md).

Fixture results against the upstream Smithy corpus:

| Category | Result |
|---|---:|
| Valid fixtures parse | 100 / 100 |
| Valid non-2.1 fixtures assemble | 81 / 81 |
| Invalid fixtures matched by parser errors | 180 / 207 |
| Semantic invalid fixtures matched by assembly/validation | 27 / 27 |

The parser-only skips are documented in
[`testdata/loader/CLASSIFICATION.md`](testdata/loader/CLASSIFICATION.md).

## Quick start

```sh
make build
./bin/gosmithy version
./bin/gosmithy parse testdata/loader/valid/main-test.smithy
./bin/gosmithy validate testdata/weather.smithy
./bin/gosmithy model --json testdata/weather.smithy | head
./bin/gosmithy generate --service example.weather#Weather \
    --generator go --module example.com/weather --out ./gen testdata/weather.smithy
(cd gen && go build ./... && go test ./...)

# HTTP+JSON client and server
./bin/gosmithy generate --service example.library#Library \
    --generator go --protocol gosmithy.protocols#httpJson \
    --module example.com/library --out ./gen-http testdata/library.smithy
(cd gen-http && go build ./...)
```

`make` builds `bin/gosmithy` with `-trimpath` and injects the version from
`git describe` (falls back to `0.1.0-dev`). Override it with
`make build VERSION=1.2.3`, or use `go build ./cmd/gosmithy` directly.

`parse` prints a debug JSON view of the AST, including line/column source
locations. `validate` exits 1 when any DANGER event is reported, and prints
events in the form `file:line:column: SEVERITY: message`. `model --json` prints
a deterministic debug view of the assembled semantic model (not a Smithy JSON
AST conformance surface); pass `-prelude=false` to omit prelude shapes.

```console
$ ./bin/gosmithy validate testdata/broken.smithy
testdata/broken.smithy:6:5: DANGER: Target `example.weather#MissingShape` is not found in the model
$ echo $?
1
```

Example:

```console
$ make build >/dev/null
$ ./bin/gosmithy parse testdata/loader/valid/main-test.smithy
{
  "filename": "testdata/loader/valid/main-test.smithy",
  "version": "2.0",
  "namespace": "example.weather",
  "shapes": [
    {
      "kind": "service",
      "name": "Weather",
      "id": "example.weather#Weather",
      ...
    }
  ]
}
```

## Model API

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

Assembly never fails on invalid user models: problems are recorded as
diagnostics and returned by `validation.Validate`. Member shapes are
first-class entries in the model (`Foo$bar`), so member traits and traversal
are uniform with other shapes. The model is frozen for codegen: shapes,
members, traits, and metadata are iterated deterministically.

`model.Node` is a resolved node value. Syntactic shape IDs (unquoted
identifiers in node values) are resolved to absolute shape IDs and marked with
`Node.SyntacticShapeID`; `validation` reports unresolvable ones with the
`SyntacticShapeIdTarget` event.

### Known M2 limitations

- IDL 2.1 semantics are not implemented. Inline collection targets are parsed
  and their references are resolved for diagnostics, but synthetic collection
  shapes are not created. The 18 valid 2.1 fixtures are parsed, not assembled.
- Selector-based trait validation, custom validators, suppressions, and
  severity overrides are not implemented. A set of cheap, explicit trait target
  checks is applied instead.
- There is no JSON AST loader. The Java repository's shared fixture is ported
  to `testdata/shared.smithy` for tests.
- No model transforms or projections.

## Code generation

Milestones 3 and 4 add the plugin contract, the Go target, and the HTTP+JSON
protocol. A generator receives one request and returns files; it never writes
to disk:

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

`codegen.Runner` calls the selected generators with the same request, merges
their files, rejects paths outside the output directory, detects duplicate
paths with differing content, and writes only after every generator succeeds.
DANGER diagnostics abort the run.

```console
$ ./bin/gosmithy generate --service example.weather#Weather \
    --generator go --module example.com/weather --out ./gen \
    testdata/weather.smithy
7 file(s) written, 0 skipped, 0 diagnostic(s)

$ find gen -type f | sort
gen/go.mod
gen/internal/smithy/json.go
gen/internal/smithy/validate.go
gen/types/enums.go
gen/types/types.go
gen/types/unions.go
gen/types/validate.go
```

The generated module uses only the standard library. The `go` generator maps
Smithy simple shapes to Go scalars, structures to structs (optional members are
pointers or nilable slices/maps), lists/sets and maps to named collection types,
enums/intEnums to named string/int32 types with constants and `Values()`,
unions to an interface plus wrapper structs, and errors to structs implementing
`error`, `ErrorCode()`, and `ErrorFault()`. Every structure, error, and named
collection has a `Validate()` method covering `@required`, `@length`, `@range`,
`@pattern`, `@uniqueItems`, and nested shapes. Member constraints override the
same constraint on the target shape; nested failure paths look like
`nested.field`, `items[0]`, or `index[key]`.

`--list-generators` lists the registry, and repeatable `--opt key=value` flags
pass generator-specific options. `testdata/validators.smithy` exercises every
supported type and constraint; `testdata/library.smithy` is a realistic example
with enums, an intEnum, a union, errors, lists, maps, and HTTP bindings; and
`testdata/weather.smithy` is the smallest demo model.

### HTTP+JSON clients and servers

Selecting `--protocol gosmithy.protocols#httpJson` adds `service.go`,
`client.go`, `server.go`, `protocol.go`, and the HTTP runtime. Operations use
`@http` plus the standard `@httpLabel`, `@httpQuery`, `@httpQueryParams`,
`@httpHeader`, `@httpPrefixHeaders`, `@httpPayload`, `@httpResponseCode`, and
`@httpError` traits. Unbound members are JSON-body fields; `@jsonName`,
`@mediaType`, and all four timestamp formats are honored. The generated server
validates input and maps validation failures to 400 `ValidationException`;
unknown routes, wrong methods, malformed bodies, body limits, modeled errors,
and unhandled errors all use the documented JSON error envelope. See
[`docs/http-json-protocol.md`](docs/http-json-protocol.md) for the protocol
guide and generated API.

`testdata/http.smithy` exercises every binding and error path. The Go generator
tests generate it into a temporary module and run an `httptest` round trip for
labels, query parameters, headers, prefix headers, JSON bodies, blob payloads,
unions, big numbers, timestamps, modeled errors, unknown errors, validation
failures, and empty operations.

### Known limitations

- `@default` and `@clientOptional` are parsed but not applied to types or
  values yet.
- Union values are not validated; map keys are not validated (map values are).
- Validators do not detect cycles: callers must not build cyclic values.
- `@pattern` uses Go's RE2 syntax. Patterns that RE2 cannot compile fail
  generation with a clear error.
- The HTTP+JSON protocol has no AWS compatibility, signing, authentication,
  retries, streaming, event streams, compression, or multipart support.
- Prefix-header suffix keys are lowercased because HTTP header names are
  case-insensitive.
- There is no JSON AST loader; the M4 follow-up is described in
  [`docs/plan/04-http-json-protocol.md`](docs/plan/04-http-json-protocol.md).
- `@length` counts Unicode code points for strings and bytes for blobs.
- `@range` on `bigInteger` and `bigDecimal` is not generated.
- IDL 2.1 inline collection targets are not generated yet because M2 does not
  create their synthetic collection shapes.

## Parser API

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

Use `idl.ParseWithOptions` to pin the supported/newest IDL version:

```go
file, err := idl.ParseWithOptions("weather.smithy", src, idl.Options{
    Version:        idl.V2_0, // newest version the caller supports
    DefaultVersion: idl.V2_0, // used when the file has no $version
})
```

`Version` gates 2.1 syntax. `DefaultVersion` controls what a file without a
`$version` control statement is assumed to be. The CLI supports 2.1 and, per
`PLAN.md`, assumes 2.0 when a file omits `$version`.

## Packages

```text
cmd/gosmithy/   CLI: parse, validate, model, generate, version, help
codegen/        generator contract, registry, runner, conflict detection
codegen/golang/ Go target: naming, type mapping, types, validators, tests
internal/runtime/ source for the generated internal/smithy package
idl/            tokenizer, AST, parser, fixture and fuzz tests
model/          semantic model, prelude, assembly, mixins, service closure
validation/     structural validators and events
testdata/       fixture corpus, weather and validators models, golden codegen output
docs/plan/      one markdown per milestone
```

## Make targets

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

## Tests

```sh
make check   # gofmt check + go vet + go test
```

Or run the underlying commands directly:

```sh
gofmt -l .
go vet ./...
go test ./...
```

The fixture harness lives in `idl/fixtures_test.go`:

- every valid fixture must parse;
- every syntax-level invalid fixture must produce the expected error
  (normalized substring match, mirroring the upstream Java harness);
- the semantic invalid fixtures are exercised by `model/` and `validation/`
  tests, which assert the event ID, message, and source location.

The semantic model tests also assemble every non-2.1 valid fixture and compare
flattened mixin models against the upstream `.flattened.smithy` fixtures.

The `codegen/golang` tests generate the weather and validator fixtures, compare
them against `codegen/golang/testdata/golden/<case>`, compile and vet the
generated modules, and run generated validator behavior tests in a temporary
module. Update golden output intentionally with:

```sh
go test ./codegen/golang -run TestGolden -update
```

A short fuzz smoke run:

```sh
make fuzz                  # 10s by default
make fuzz FUZZTIME=30s
```

## Plan

The overview is in [PLAN.md](PLAN.md). Each milestone is one unit of work and
one document:

1. [Parser foundation](docs/plan/01-parser-foundation.md) — skeleton, fixture
   harness, tokenizer, parser, AST. **Implemented.**
2. [Semantic model](docs/plan/02-semantic-model.md) — resolution, prelude,
   mixins, closures, structural validation. **Implemented.**
3. [Codegen core + Go models](docs/plan/03-codegen-core-and-go.md) — plugin
   contract, Go types and validators. **Implemented.**
4. [Generic HTTP+JSON](docs/plan/04-http-json-protocol.md) — `httpJson`
   protocol, generated client and server. **Implemented.**

## Attribution

The fixture corpus under `testdata/loader/` is derived from the Smithy Java
repository and is licensed under Apache-2.0. See
[`testdata/NOTICE`](testdata/NOTICE) and [`testdata/LICENSE`](testdata/LICENSE).
The embedded prelude in [`model/prelude.smithy`](model/prelude.smithy) is based
on `smithy-model/src/main/resources/.../prelude.smithy` in the same repository,
with one gosmithy extension (`EPOCH_MILLISECONDS` in `timestampFormat`), and is
licensed under Apache-2.0.
