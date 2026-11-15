# Milestone 2 — Semantic model and structural validation

Status: implemented · Estimate: **7 dev-days** · Depends on: Milestone 1
Deliverable: `gosmithy validate` and `gosmithy model` over real Smithy models.

> **Implementation note (2026-11-14).** The semantic model, prelude, assembler,
> mixin flattening, service closure, structural validators, and CLI commands are
> implemented. All 100 valid fixtures assemble with no DANGER events, including
> the 19 that use 2.1 syntax or version, and all 27 semantic invalid fixtures
> produce the expected event category at the expected source location. Mixin
> flattening is additionally compared against every upstream `.flattened.smithy`
> fixture.
>
> Two implementation notes differ from the sketch below:
>
> - `validation.Event` is an alias of `model.Diagnostic`. This keeps the
>   dependency direction `idl → model → validation` without an import cycle;
>   the assembler can record diagnostics while validation adds checks.
> - The 2.1 semantics described in
>   [Milestone 5](05-smithy-2.1-semantics.md) were added after this milestone:
>   inline collections create synthetic shapes and member indexes apply
>   `smithy.protocols#idx`.
>
> Event IDs are stable constants in `validation`: `Model.UnresolvedShape`,
> `Model.InvalidMember`, `Model.InvalidMixin`, `Model.InvalidEnum`,
> `SyntacticShapeIdTarget`, and so on. `gosmithy validate` exits 1 when any
> DANGER event survives severity filtering.

---

## 1. Unit of work

Turn the source-located AST from Milestone 1 into the **semantic model**:
resolved shape IDs, first-class member shapes, applied traits, the prelude,
mixins, multi-file merging, and the service closure. Add the structural
validation needed to make that model trustworthy.

A parser that cannot resolve `Foo` against a namespace, the prelude, or a `use`
statement is not usable. Resolution and structural validation are therefore one
unit; there is no useful way to ship one without the other.

**Demo**

```sh
$ go run ./cmd/gosmithy validate testdata/weather.smithy
# no output, exit 0

$ go run ./cmd/gosmithy validate testdata/broken.smithy
broken.smithy:12:5: DANGER: Target `MissingShape` is not found in the model
# exit 1

$ go run ./cmd/gosmithy model --json testdata/weather.smithy
{ "shapes": { "example.weather#Weather": { ... } } }
```

---

## 2. Scope

### In

- `model.Node`: null/bool/number/string/array/object, source locations,
  syntactic shape-ID resolution.
- `model.ShapeID`: parse, format, absolute/relative/member IDs, case-insensitive
  conflict detection.
- `model.Shape`: simple, aggregate, enum, intEnum, service, resource,
  operation, member; traits; source locations.
- `model.Trait`: generic applied trait (`ID → Node`) plus typed accessors for
  the traits used in M2/M3/M4.
- `model.Model`: shape lookup, metadata, iteration in deterministic order.
- Prelude: embed and parse the real
  `smithy-model/src/main/resources/.../prelude.smithy`; no hand-maintained
  duplicate list of built-in shapes.
- Assembly:
  - multiple files, metadata merging;
  - namespaces, `use` statements, relative ID resolution;
  - forward references (shapes and traits);
  - `apply` statements, including forward targets;
  - merge of duplicate shape definitions and trait conflict rules;
  - mixins (data shapes, services, resources, operations);
  - operation inline input/output synthesis;
  - default `Unit` input/output for operations;
  - enum/intEnum sugar and member values;
  - service/resource properties and operation collection;
  - service closure.
- Structural validation (see §4).
- CLI:
  - `validate` with severity filtering and exit codes;
  - `model --json` (debug serializer, not a JSON AST conformance claim);
  - keep `parse`.
- Tests against the M1 fixture corpus plus focused semantic fixtures.

### Out

- Full spec validation (selectors, custom validators, suppressions, severity
  overrides, 78 built-in validators).
- JSON AST **loader**.
- Code generation.
- Protocol selection or HTTP binding validation.
- Model transforms/projections.
- IDL 2.1 semantics (added later in Milestone 5).

---

## 3. Assembly design

Three phases, in order. Resist merging them.

### Phase A — Parse

- One `idl.File` per source.
- No cross-file resolution.
- Every unresolved shape reference remains a string.
- Syntactic shape-ID string nodes are marked (`Node.SyntacticShapeID`).

### Phase B — Build and resolve

1. Create a skeleton `Shape` for every root shape definition.
2. Create member shapes (`Foo$bar`, `List$member`, `Map$key`, `Map$value`,
   enum members) and link `Container`/`Target`.
3. Resolve root and member references:
   - current namespace;
   - `use` aliases (root and member);
   - public prelude shapes;
   - forward references (deferred until all declarations exist).
4. Resolve syntactic shape-ID string nodes to absolute IDs.
5. Resolve `apply` targets and collect pending traits.

### Phase C — Apply, flatten, merge, validate

1. Apply traits now that all trait definitions are known.
2. Coerce annotation trait values using the trait's own shape:
   - structure/map trait without a body → `{}`;
   - list trait without a body → `[]`;
   - scalar trait without a body → parse error.
3. Flatten mixins into concrete shapes (members + traits), preserving
   deterministic order and reporting conflicts.
4. Merge duplicate shapes across files; merge metadata.
5. Build indexes (service closure, operation map).
6. Run structural validation.
7. Freeze the model for codegen (deterministic iteration order).

### Prelude

- Embedded with `go:embed` from `model/prelude.smithy`.
- Parsed by our own parser, assembled first, in namespace `smithy.api`.
- Relative IDs fall back to **public** prelude shapes only (`@private` shapes
  are not resolvable by name).
- If the prelude fails to load, that is a fatal internal error, not a user error.

### Mixins (MVP semantics)

- A mixin is a shape with `@mixin`.
- `with [A, B]` on data shapes, services, resources, and operations.
- Elided members (`$foo`) copy the mixin member's target and traits; traits on
  the elided member override/add to the mixin's member traits.
- After flattening, concrete shapes contain all members; keep the original
  `Mixins` list for diagnostics.
- Validate:
  - mixin and target are compatible shape kinds;
  - every elided member exists on a mixin;
  - no duplicate resulting member names;
  - mixin members do not introduce illegal targets.
- Mixins of mixins are flattened transitively; cycles are a DANGER.

### Inline operation I/O

- `input := { ... }` / `output := { ... }` create synthetic shapes:
  - default names `<OperationName>Input` / `<OperationName>Output`;
  - suffixes configurable via `$operationInputSuffix` /
    `$operationOutputSuffix`;
  - `@input` / `@output` traits applied;
  - source location = the operation.
- Explicit `input: Foo` uses `Foo` as-is.
- Missing input/output → `smithy.api#Unit`.
- The synthetic shape is not silently added to the service closure unless it is
  referenced (it is, via the operation, so it is in the closure).

### Service closure

Traverse from the service shape:

- `service.operations`;
- `service.resources` recursively;
- resource `operations`, `collectionOperations`, and lifecycle operations
  (`create`, `read`, `update`, `delete`, `list`, `put`);
- operation input/output/errors;
- member targets of every shape in the closure;
- service-level errors (they are in the closure and are effective errors for
  every operation, but do not mutate the operation's modeled error list).

Do not include trait-only references in MVP closure. Keep closure logic in one
file (`model/closure.go`) and test it independently.

### Merging

- Metadata: arrays concatenate; equal values agree; otherwise DANGER.
- Shapes with the same ID: same type; aggregates have the same members and
  targets; services have the same properties; traits merge.
- Traits: list/set values concatenate; equal values agree; otherwise DANGER.
- Case-insensitive shape ID/member conflicts are DANGER.
- Source locations: keep the first definition; report the conflict against the
  duplicate.

---

## 4. Structural validation (minimum)

Implement these as small, testable checks over the assembled model. This is not
the full validator framework; those checks come later.

| Check | Event ID (suggested) |
|---|---|
| Shape ID syntax and duplicate IDs (case-insensitive) | `Model.DuplicateShapeId` |
| Unresolved shape/member reference | `Model.UnresolvedShape` |
| Member target type is legal (no operation/resource/service/member/trait) | `Model.InvalidMemberTarget` |
| Duplicate member names / invalid member names | `Model.InvalidMember` |
| Trait target not found | `Model.UnresolvedTrait` |
| Applied value not a trait (`@trait` shape) | `Model.NotATrait` |
| Annotation trait value has the wrong shape | `Model.InvalidTraitValue` |
| Operation input/output exists and is a structure or `Unit` | `Model.InvalidOperationIo` |
| Operation errors target `@error` structures | `Model.InvalidOperationError` |
| Service/resource references resolve to the right types | `Model.InvalidReference` |
| Resource identifiers/properties/operations resolve | `Model.InvalidResource` |
| Mixin compatibility, cycles, missing elided members | `Model.InvalidMixin` |
| `@enum` / enum member values are valid and unique | `Model.InvalidEnum` |
| Constraint trait shape/value (length, range, pattern) | `Model.InvalidTraitValue` |
| Required application targets (members only, etc.) | `Model.InvalidTraitTarget` |
| Syntactic shape ID that does not resolve | `SyntacticShapeIdTarget` (DANGER, per spec) |

Design notes:

- `validation.Event` has `ID`, `Severity` (NOTE/WARNING/DANGER), `Message`,
  `ShapeID`, `SourceLocation`.
- Validation returns all events; CLI decides exit code (DANGER → 1).
- Trait selector enforcement is **not** in MVP; do not fake it. Apply only
  cheap, explicit target checks where codegen depends on them.
- No custom validators, suppressions, or severity overrides yet.

---

## 5. Work breakdown

| Unit | Work | Estimate |
|---|---|---|
| U1 | `Node`, `SourceLocation`, `ShapeID`, parse/format/resolve; unit tests | 1.0d |
| U2 | `Shape`, `Member`, `TraitMap`, `Model`; deterministic iteration; typed trait accessors | 1.0d |
| U3 | Prelude embed/parse/assemble; public-shape resolution | 0.75d |
| U4 | Assembler: skeleton build, member creation, references, forward refs, apply, trait coercion, metadata merge | 2.0d |
| U5 | Mixins, inline I/O, operation defaults, service closure | 1.25d |
| U6 | Structural validators + events + CLI `validate` / `model --json` | 1.0d |
| U7 | Fixture runner updates, semantic fixtures, source-location tests | 0.5d |
| U8 | Documentation of model semantics/limitations | 0.5d |

### U4 details

Keep a single `assembly` struct with explicit state:

```go
type assembly struct {
    files      []*idl.File
    shapes     map[ShapeID]*Shape
    pending    []pendingRef
    traits     []pendingTrait
    metadata   map[string]Node
    diagnostics []validation.Event
}
```

Avoid global maps and init order. Resolution is deterministic: sort files by
path, shapes by ID, and pending references by source position.

### U6 details

Start validators as plain functions:

```go
func validateTargets(m *Model) []Event
func validateOperations(m *Model) []Event
func validateMixins(m *Model) []Event
```

Group them in `validation/validate.go`; no plugin registry yet. If the list
exceeds ~15 checks, introduce a `Validator` interface; not before.

---

## 6. Acceptance criteria

- [x] `go test ./...` and `go vet ./...` pass.
- [x] Prelude loads from the embedded real `prelude.smithy`.
- [x] The weather example assembles; shape IDs, members, input/output, errors,
      and resource lifecycle operations resolve as expected.
- [x] All valid fixtures assemble, including the 2.1 fixtures added in
      Milestone 5. The one shared `__shared.json` fixture is ported to an IDL
      helper for tests (no JSON AST loader yet).
- [x] All semantic invalid fixtures classified in M1 emit the expected event
      category and source location.
- [x] `gosmithy validate` exits 0 on valid models and 1 on DANGER.
- [x] `gosmithy model --json` is deterministic across runs.
- [x] Every member shape is reachable from its container and has a source
      location.
- [x] No codegen packages imported by `model`.
- [x] No AWS-specific code or trait names in `model`/`validation`.

**Demo**

```sh
go run ./cmd/gosmithy validate testdata/weather.smithy
go run ./cmd/gosmithy model --json testdata/weather.smithy | head
```

---

## 7. Risks

| Risk | Mitigation |
|---|---|
| Forward references and `use` resolution order | Two-phase assembly; pending refs; test each case from `valid/forwardrefs/`. |
| Mixin semantics diverge from Java | Start with data shapes + explicitly test service/resource/operation mixins from fixtures; document limits. |
| Prelude bootstrap (traits defined in prelude applied to prelude shapes) | Apply traits in phase C, after all definitions; never require trait definitions during parse. |
| Fixture dependence on JSON AST shared model | Port `__shared.json` to IDL for tests; defer JSON AST loader. |
| Validator scope creep | Only checks codegen depends on; no selector engine. |
| Source-location quality | Assert file/line/column in tests; make `Event` rendering part of the CLI. |

---

## 8. Deliverables

- `model` package with a working semantic model and prelude.
- `validation` package with structural events.
- `gosmithy validate` and `gosmithy model`.
- Semantic fixture tests and a documented limitation list.
- The stable model API that Milestone 3's generator consumes.
