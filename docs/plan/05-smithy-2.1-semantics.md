# Smithy 2.1 semantics — Plan A

Status: implemented · Estimate: **2–3 dev-days** · Depends on: Milestones 1–4

Deliverable: inline collections assemble into synthetic shapes, member indexes
apply the `idx` trait (with a built-in `smithy.protocols#idx` definition), all
19 2.1-versioned fixtures assemble, and the README states feature coverage and
non-coverage at a glance.

Implementation notes:

- `model/synthetic.go` creates and groups synthetic shapes. The built-in `idx`
  trait lives in `model/protocols.smithy` and is loaded by `model.Assemble`
  unless the user model declares the same shape; it is independent of the CLI
  `--protocol`/`-protocols` switch.
- The resolved-root pre-scan in `model/assemble.go` makes local shapes shadow
  the prelude for forward references, matching upstream.
- Go codegen emits synthetic collections as named types. If a user shape
  already owns the preferred Go name, the synthetic type gets a numeric suffix
  instead of a generation error.

## 1. Scope

Implement the two missing 2.1 semantics:

1. **Inline collections** — create synthetic list/map shapes using the
   upstream content-derived naming (`_SyntheticListOf…`, `_SyntheticMapOf…`),
   mark them `smithy.api#synthetic`, group identical collections by namespace,
   support nesting, and reject user-defined collisions and traits on synthetic
   shapes.
2. **Member index shorthand** — apply the absolute `smithy.protocols#idx`
   trait, including elided members. Embed a minimal `smithy.protocols#idx`
   trait definition so shorthand works without the user supplying it. If the
   user model already defines that ID, the user definition wins and no
   duplicate is injected. Do not reorder members and do not enforce `idx`
   contiguity/range (`indexed` is a separate protocol-traits validator).

Tagged literals already work; the existing `Inline` model field and the
codegen DANGER are replaced by real synthetic collection shapes.

Out of scope: `indexed` service validation, the rest of the
`smithy-protocol-traits` model, selectors, JSON AST loader, and full Smithy
conformance.

## 2. Implementation outline

- Pre-scan declared roots so forward references shadow the prelude correctly
  (needed for the local-target fixture).
- Add synthetic naming/construction, deduplication, collision diagnostics,
  and the synthetic-trait restriction.
- Apply `smithy.protocols#idx` from `idl.Member.Index`.
- Embed the minimal `idx` trait definition and load it with the tool's
  built-in model definitions; detect a user-supplied `smithy.protocols#idx`
  before assembly and prefer it. Keep this independent of the `--protocol`
  selector and `-protocols=false`, which control the wire protocol definition.
- Add tests for both paths: shorthand without a local `idx` definition
  validates cleanly, and the upstream `member-indexes` fixture (which defines
  `idx` locally) still assembles without duplicate-shape or trait conflicts.
- Remove inline special cases from validation, model JSON, and Go codegen.
- Emit synthetic collections as named Go collection types with collision-safe
  names.
- Unskip all 19 2.1 fixtures and add focused assertions for the upstream
  synthetic IDs, grouping, nesting, union/use cases, and idx values.
- Add a compile test for a service using inline collections.
- Update `PLAN.md`, the M2 note, `README.md`, and `CLASSIFICATION.md` (19, not
  18, 2.1 fixtures).

## 3. README feature coverage (new deliverable)

Add a **Smithy feature coverage** section to `README.md` with two clearly
separated lists:

### Covered

Use `- [x]` checkboxes, grouped by area, not a flat spec dump:

- [x] IDL 2.0 grammar and IDL 2.1 syntax: inline collections, member indexes,
      tagged literals.
- [x] Assembly: prelude, namespaces/`use`, `apply`, forward references,
      mixins, inline operation I/O, duplicate merging, service closure.
- [x] Synthetic 2.1 semantics: inline list/map shapes, `@synthetic`, `idx`.
- [x] Structural validation: unresolved targets, invalid members, operation
      I/O, mixins, enums, syntactic shape IDs.
- [x] Go codegen: types, validators, enums, unions, errors, and `httpJson`
      clients/servers.
- [x] Self-contained generated modules using only the standard library.

### Not covered

Use plain high-level bullets, grouped into a few large areas. No exhaustive
spec checklist:

- **AWS**: protocols, auth/signing, endpoints, SigV4, AWS traits.
- **Full validation**: selector engine, 78-validator suite, suppressions,
  severity overrides, model transforms.
- **Other input formats**: JSON AST loader, `smithy-build.json`, projections,
  plugin/Maven resolution.
- **Other wire features**: streaming, event streams, XML, CBOR, multipart,
  retries, compression.
- **Other targets**: non-Go code generators and the Java codegen framework.
- **Protocol-traits breadth**: `indexed` service validation and `idx`
  range/contiguity; only the minimal `idx` trait definition is built in.

The section should link to the detailed limitations rather than duplicate
them, and the fixture-results table should be updated to show 100/100 valid
fixtures assembling after this work.

## 4. Acceptance

- `TestValidFixturesAssemble` reports **100 assembled, 0 skipped**.
- Inline-collection fixtures match the upstream synthetic shape IDs, targets,
  grouping, and `@synthetic` trait.
- `member-indexes` carries the expected `smithy.protocols#idx` values.
- A 2.1 member index validates without a local `idx` definition, and the
  locally defined `idx` fixture still assembles cleanly.
- Invalid inline targets still produce one `Model.UnresolvedShape` at line 7.
- A generated service with inline collections builds.
- README has the covered/not-covered section described above.
- `make check` is green.
