# Milestone 3 — Codegen core and Go models/validators

Status: implemented · Estimate: **6 dev-days** · Depends on: Milestone 2
Deliverable: a pluggable generator contract and the first real target: Go types
and validators.

> **Implementation note (2026-11-14).** The `codegen` contract, registry,
> runner, CLI `generate`, Go generator, embedded `internal/smithy` runtime,
> golden tests, generated-module compile tests, and generated validator
> behavior tests are implemented. Two fixtures cover the surface:
> `testdata/weather.smithy` for the demo and `testdata/validators.smithy` for
> every supported constraint and Go type.
>
> Small deviations from the sketch below, all internal:
>
> - `Runner.Run` returns a `*RunResult` as well as an error so the CLI can print
>   an exact written/skipped/diagnostic summary. DANGER diagnostics still abort
>   before any file is written.
> - The generator receives its module path through `Request.Options["module"]`,
>   matching the `--opt` mechanism the contract already defined.
> - Named collections get `Validate` and an unexported `validateContents`
>   helper. The helper lets a member override the target collection's own
>   constraints without losing element validation.
> - `types.Unit` is always emitted so Milestone 4 can use `*types.Unit` in
>   operation signatures.
> - Union validation is intentionally omitted, per §4.8. Map keys are not
>   validated; values are.

---

## 1. Unit of work

Introduce the codegen plugin contract and implement the first Go generator
against it. The contract and the first plugin are one unit: an abstract plugin
interface with no real generator is unproven, and a generator with no contract
is not pluggable.

This milestone produces a real, useful artifact: a compiled Go module containing
the service's data model and validation methods. It does **not** produce clients
or servers; those belong to Milestone 4.

**Demo**

```sh
$ go run ./cmd/gosmithy generate \
    --service example.weather#Weather \
    --generator go \
    --module example.com/weather \
    --out ./gen \
    testdata/weather.smithy

$ cd gen && go build ./... && go test ./...
```

Generated tree:

```
gen/
  go.mod
  types/
    types.go        # structures, lists, maps, unit, errors
    enums.go        # enum and intEnum types + constants
    unions.go       # union interface + member wrappers
    validate.go     # Validate() methods
  internal/smithy/
    validate.go     # ValidationError(s), used by generated validators
```

---

## 2. Scope

### In

- `codegen` package:
  - `Generator` interface;
  - `Request` / `Response` / `File` / `Diagnostic`;
  - `Registry` and `Runner`;
  - deterministic file merging and path-conflict detection;
  - CLI `generate` with `--generator`, `--service`, `--module`, `--out`,
    `--opt key=value`, and `--list-generators`.
- Go target (`codegen/golang`):
  - naming rules, reserved words, service renames, collision handling;
  - type mapping and member nullability;
  - structures, lists, maps, sets, unit;
  - enums and intEnums with constants;
  - unions as interface + wrapper structs;
  - error structures implementing `error` and `ErrorCode()`;
  - doc comments from `@documentation` / `///`;
  - `Validate()` methods for required, length, range, pattern, uniqueItems,
    and nested shapes;
  - deterministic output, `go/format`, import management.
- Embedded `internal/smithy` runtime source with validation error types.
- Generated `go.mod` and package layout.
- Golden tests, compile tests, and generated validator behavior tests.

### Out

- Client, server, protocol, HTTP bindings.
- JSON encoding/decoding (Milestone 4).
- `@default` wire semantics, `@clientOptional` input/output variants.
- Service/resource client abstractions, pagination helpers.
- Streaming.
- AWS-specific anything.
- External (out-of-process) plugins; the in-process contract only.

---

## 3. Plugin contract

The contract is deliberately small and Buf-like: a generator receives one
request and returns files; it does not write to disk or read global state.

```go
package codegen

type Generator interface {
    Generate(ctx context.Context, req *Request) (*Response, error)
}

type Request struct {
    Model    *model.Model
    Service  model.ShapeID
    Version  model.Version      // 2.0 for MVP
    Target   Target             // "go"
    Protocol *Protocol          // nil in M3
    Options  map[string]string
    Files    []string           // source files, diagnostics only
}

type Response struct {
    Files       []File
    Diagnostics []Diagnostic
}

type File struct {
    Path    string      // relative to --out, slash-separated
    Content []byte
    Mode    fs.FileMode
}

type Diagnostic struct {
    Severity validation.Severity
    Message  string
    Path     string
    Line     int
    Column   int
}
```

Registry and runner:

```go
type Factory func() Generator

type Registry struct{ /* ... */ }
func (r *Registry) Register(name string, f Factory)
func (r *Registry) Get(name string) (Generator, bool)

type Runner struct {
    OutDir string
    DryRun bool
}
func (r *Runner) Run(ctx context.Context, req *Request, gens []Generator) error
```

Runner behavior:

- Call each generator with the same request.
- Merge all `File`s; reject absolute paths, `..`, duplicate paths with differing
  content.
- Create directories, write files atomically (temp + rename).
- Print diagnostics; DANGER diagnostics fail the run.
- No formatting in the runner; generators format their own output.
- Stable summary: files written, skipped, diagnostics.

Why this shape:

- Testable: a fake generator returns files; the runner's merge/conflict logic is
  unit-testable.
- Composable: several generators can write into one module.
- Extensible: the same request/response can later be serialized over
  stdin/stdout for external plugins, without changing the interface.

Targets are bundles of generators:

```go
type Target struct {
    Name       string
    Generators []string
}
```

MVP registry contains one generator, `"go"`. The CLI also accepts
`--generator go` for now; a `--target` alias can be added when there is more than
one.

---

## 4. Go generation rules

### 4.1 Naming

- Shape name → PascalCase (`my_shape` → `MyShape`, `httpURL` → `HttpURL`).
- Member name → PascalCase (`content_type` → `ContentType`).
- Service `rename` map applies before casing; renames are shape-scoped.
- Reserved words and generated names get a trailing `_` (`Type` is fine; `Error`,
  `String`, `Service`, `Client`, `Handler`, `ValidationError` are reserved in the
  generated package if they would collide).
- Enum constants: `<EnumName><ValueName>`; sanitize to identifier characters,
  ensure uniqueness, fall back to the raw value if needed.
- Union wrappers: `<UnionName><MemberName>`.
- Error types are the shape name; no extra suffix.
- All exported names are checked against Go keywords and the package's generated
  names; collisions are a DANGER diagnostic, not silent renames.

### 4.2 Type mapping

| Smithy | Go |
|---|---|
| `string` | `string` |
| `enum` | named `string` |
| `intEnum` | named `int32` |
| `blob` | `[]byte` |
| `boolean` | `bool` |
| `byte` | `int8` |
| `short` | `int16` |
| `integer` | `int32` |
| `long` | `int64` |
| `float` | `float32` |
| `double` | `float64` |
| `bigInteger` | `*big.Int` |
| `bigDecimal` | `*big.Float` |
| `timestamp` | `time.Time` |
| `document` | `any` |
| `list` / `set` | `[]T` |
| `map` | `map[string]V` |
| `structure` | `struct` |
| `union` | interface + wrapper |
| `Unit` | `struct{}` |
| error | struct + `error` |

### 4.3 Member nullability (MVP)

All structure/union members that can be absent are pointer-like; validators use
nil as "absent". This keeps `@required` meaningful (empty string ≠ absent).

- scalar, enum, intEnum, structure, union member → `*T`;
- list/set, map, blob, document → slice/map/`[]byte`/`any` (nil means absent);
- requiredness is not encoded in the Go type; it is enforced by `Validate()`;
- list/map **elements** are values unless the collection is `@sparse`;
- `@default` is parsed but not applied to types/wire in MVP; document it.

This is deliberately simpler than AWS's client/server nullability variants. One
type per structure, no input/output variants.

### 4.4 Structures

```go
// A city.
type City struct {
    // The city ID.
    CityID *string `json:"cityId,omitempty"`
    Name   *string `json:"name,omitempty"`
}
```

- Fields sorted by Smithy member declaration order? For stable output, sort by
  member name is simpler; but docs/readability favor declaration order. Use
  declaration order (model preserves it) and enforce deterministic tie-breaks.
- JSON tags use the Smithy member name by default; `@jsonName` is a protocol
  concern (M4), but emitting the tag now is harmless. Keep the Smithy name in
  the tag in M3; M4's protocol can override.
- Documentation: preceding `///` and `@documentation` become Go comments.
- Deprecated: `Deprecated:` line in the doc comment (`@deprecated`).

### 4.5 Enums and intEnums

```go
type Color string

const (
    ColorRed   Color = "red"
    ColorGreen Color = "green"
)

func (Color) Values() []Color { ... }
```

- Enum shape members have names and string values; intEnum members have names
  and int32 values.
- `@enum` on a string shape is still supported in M2; M3 generates the same
  named-string form. (The shape type is `string` with `@enum`, not `enum`.)
- Unknown values: no `Unknown` constant; users can cast.

### 4.6 Unions

```go
type Shape interface{ isShape() }

type ShapeCircle struct{ Value *Circle }
type ShapeSquare struct{ Value *Square }

func (*ShapeCircle) isShape() {}
func (*ShapeSquare) isShape() {}
```

- Simple members use value fields; structures/unions/lists/maps use pointers.
- No `Unknown` member in MVP.
- A nil union interface is "not set"; validation handles requiredness.

### 4.7 Errors

```go
type NotFound struct {
    Message *string `json:"message,omitempty"`
    Resource *string `json:"resource,omitempty"`
}

func (e *NotFound) Error() string { ... }
func (e *NotFound) ErrorCode() string { return "NotFound" }
func (e *NotFound) ErrorFault() string { return "client" }
```

- `Error()` uses `Message`/`message` if present, else the shape name.
- `@error` fault string is preserved for protocol mapping (M4).
- Error structures also get `Validate()` if they have constraints.

### 4.8 Validators

Generate `Validate() error` on every structure, union wrapper? (no), named
list/map, and error shape in the service closure.

Runtime:

```go
package smithy

type ValidationError struct {
    Path    string
    Message string
}

type ValidationErrors []ValidationError

func (e *ValidationErrors) Add(path, message string)
func (e *ValidationErrors) AddNested(path string, err error)
func (e ValidationErrors) Err() error // nil if empty
```

Generated pattern:

```go
func (v *GetCityInput) Validate() error {
    var errs smithy.ValidationErrors
    if v.CityID == nil {
        errs.Add("cityId", "is required")
    } else if len(*v.CityID) < 1 {
        errs.Add("cityId", "must be at least 1 character")
    }
    return errs.Err()
}
```

Rules:

- Member constraints override target shape constraints (Smithy semantics).
- Apply constraints to: required, length, range, pattern, uniqueItems.
- Nested: if a member target has a generated validator, call it and merge with
  the member path. Collections iterate with `[i]`/`[key]` paths.
- Avoid duplicate compile of regexes: emit one package-level `regexp.MustCompile`
  per distinct pattern.
- `uniqueItems`: nested loop with `reflect.DeepEqual` for MVP; document the
  performance limitation and refine later.
- Cycles: no cycle detection in generated validators; document that callers must
  not build cyclic values. A future depth guard is possible.
- Constraint enforcement is a caller decision: the server will validate inputs
  (M4), not outputs.

Known limitations (document in generated package or docs):

- `@pattern` uses Go's RE2 regexp, which differs from ECMAScript regex in
  lookarounds/backreferences. Models using unsupported syntax produce a clear
  generation error.
- `@length` for strings counts Unicode code points; blobs count bytes.
- `@default` / `@clientOptional` not applied yet.

### 4.9 Runtime and output layout

- The generator embeds source templates for `internal/smithy`.
- Imports are module-relative: `--module example.com/weather` →
  `example.com/weather/types` and `example.com/weather/internal/smithy`.
- `go.mod`: `module <module>`, `go 1.22`, no dependencies.
- Files are formatted with `go/format`; failure is a generator error with the
  offending file.
- Output is deterministic: sort shapes by ID, members by declaration order,
  constants by member order, imports by path.

---

## 5. Work breakdown

| Unit | Work | Estimate |
|---|---|---|
| U1 | `codegen` contract, registry, runner, conflict detection, fake-generator tests | 1.0d |
| U2 | CLI `generate`, option parsing, output summary, diagnostics | 0.5d |
| U3 | Go naming, reserved words, type mapping, member nullability, imports, writer | 1.0d |
| U4 | Structures, lists/maps/sets, enums/intEnums, unions, errors, docs | 1.5d |
| U5 | Validator generation + `internal/smithy` validation runtime | 1.0d |
| U6 | `go.mod`, deterministic formatting, golden tests, compile tests | 0.5d |
| U7 | Generated validator behavior tests, README/docs | 0.5d |

### U1 details

- Keep `codegen` free of Go-specific imports.
- Test composition: two fake generators return files into one tree; duplicate
  path with identical content is allowed, differing content is an error.
- Reject paths escaping `--out`.
- `Runner` writes only after all generators succeed (all-or-nothing; a later
  milestone can add partial-failure behavior if needed).

### U3 details

- Naming lives in `codegen/golang/naming.go`, pure functions, table-driven tests.
- Keep a `reserved` set for Go keywords, predeclared identifiers, and generated
  package names.
- `goType(shape)` and `memberGoType(member)` are separate; do not conflate.
- Import tracking: a tiny `GoWriter` with `AddImport(path, alias)` and
  `Writef`; format the final file with `go/format`.

### U5 details

- Implement only the constraints listed. Everything else is a no-op with a
  documented limitation.
- Test generated validators by writing a temp module and running a generated
  `_test.go` (or by returning the validator behavior through a test harness).
- Ensure nested errors include a path such as `nested.field` and `items[0]`.

---

## 6. Acceptance criteria

- [x] `go build ./...`, `go test ./...`, `go vet ./...` pass.
- [x] `gosmithy generate` runs the `go` plugin and writes a module.
- [x] Generated module compiles with `go build ./...` and has no external deps.
- [x] Weather model generates structures, enums, unions (if present), errors,
      and validators.
- [x] Required, length, range, pattern, uniqueItems, and nested validation
      behave correctly in generated tests.
- [x] Generated output is deterministic; golden tests pass with `-update` off.
- [x] Duplicate path/conflict detection works.
- [x] No client/server/protocol code is generated.
- [x] No AWS-specific names or behavior in the generator.

**Demo**

```sh
go run ./cmd/gosmithy generate \
  --service example.weather#Weather \
  --generator go \
  --module example.com/weather \
  --out /tmp/weather-gen \
  testdata/weather.smithy
(cd /tmp/weather-gen && go build ./... && go test ./...)
```

---

## 7. Risks

| Risk | Mitigation |
|---|---|
| Plugin contract over- or under-designed | One interface, one request/response, one runner. Add external plugins only when needed. |
| Go naming collisions | Pure naming functions with table tests; DANGER diagnostic instead of silent rename. |
| Member nullability confusion | Document the pointer rule clearly; test required empty string vs absent. |
| Validator recursion/cycles | Document caller contract; add depth guard later if real usage demands it. |
| Regex incompatibility | Compile patterns during generation; fail with a clear error for unsupported syntax. |
| Hidden dependency on M4 | No JSON tags beyond simple names, no protocol imports; runtime only validation. |

---

## 8. Deliverables

- `codegen` plugin contract, registry, runner.
- `go` generator producing types and validators.
- Generated `internal/smithy` validation runtime.
- Golden/compile/behavior tests.
- The CLI surface Milestone 4 extends with client/server generators.
