# gosmithy

A Go implementation of the Smithy IDL and a pluggable code generator.

- Core is generic Smithy. No AWS protocols or traits in the MVP.
- Parse Smithy IDL 2.0 — plus the 2.1 syntax additions — into a source-located AST.
- Generate Go types, validators, and (later) HTTP+JSON clients and servers.
- Generators implement a small Buf-inspired request/response interface.
- Generated modules are self-contained and use only the standard library.

## Status

**Milestone 1 is implemented:** module skeleton, CLI, fixture harness, tokenizer,
and parser. The parser covers the full IDL 2.0 grammar and the 2.1 syntax
additions (member indexes, inline collections, and tagged string literals).
Semantic assembly, validation, and code generation are Milestones 2–4 and are
not implemented yet.

Fixture results against the upstream Smithy corpus:

| Category | Result |
|---|---:|
| Valid fixtures parse | 100 / 100 |
| Invalid fixtures matched by parser errors | 180 / 207 |
| Invalid fixtures deferred to semantic validation (M2) | 27 |

The remaining 27 are documented in
[`testdata/loader/CLASSIFICATION.md`](testdata/loader/CLASSIFICATION.md).

## Quick start

```sh
make build
./bin/gosmithy version
./bin/gosmithy parse testdata/loader/valid/main-test.smithy
```

`make` builds `bin/gosmithy` with `-trimpath` and injects the version from
`git describe` (falls back to `0.1.0-dev`). Override it with
`make build VERSION=1.2.3`, or use `go build ./cmd/gosmithy` directly.

`parse` prints a debug JSON view of the AST, including line/column source
locations. It is not a Smithy JSON AST conformance surface.

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
cmd/gosmithy/   CLI: parse, version, help
idl/            tokenizer, AST, parser, fixture and fuzz tests
testdata/loader upstream Smithy fixture corpus + classification
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
- semantic invalid fixtures are listed explicitly in
  `testdata/loader/CLASSIFICATION.md` and deferred to M2.

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
   mixins, closures, structural validation.
3. [Codegen core + Go models](docs/plan/03-codegen-core-and-go.md) — plugin
   contract, Go types and validators.
4. [Generic HTTP+JSON](docs/plan/04-http-json-protocol.md) — `httpJson`
   protocol, generated client and server.

## Attribution

The fixture corpus under `testdata/loader/` is derived from the Smithy Java
repository and is licensed under Apache-2.0. See
[`testdata/NOTICE`](testdata/NOTICE) and [`testdata/LICENSE`](testdata/LICENSE).
