# Smithy specification coverage

This document audits gosmithy against the
[Smithy 2.0 specification](https://smithy.io/2.0/spec/index.html), chapter by
chapter and section by section. It also records the Smithy 2.1 additions the
project implements and the parts of the implementation that are outside the
specification itself (the `gosmithy` CLI, code-generation contract, and Go
target).

The tree below follows the specification's table of contents. Checkboxes are
per capability, not per source file.

Legend:

- `[x]` — implemented and covered by the repository's tests (including
  compile, golden, and round-trip tests where applicable).
- `[x] **Partial**` — a usable subset is implemented; the note states what is
  missing.
- `[ ]` — not implemented, not enforced, or accepted only as an opaque generic
  trait/selector value.
- **Parsed only** — the syntax or trait is accepted and stored in the model,
  but it has no further semantic, validation, or code-generation effect.

Traits are generic in gosmithy. Unless an item says otherwise, an unimplemented
trait can still be applied in a model, is stored in `TraitMap`, and passes
through the same trait-value validation as every other trait. The checkboxes
track semantics, validation, and code generation rather than parser
acceptance. This distinction matters for almost every trait in the
specification.

## At a glance

| Specification chapter | Status | Main gaps |
|---|---|---|
| [The Smithy model](https://smithy.io/2.0/spec/model.html) | Mostly | Trait selectors/conflicts/breaking rules; duplicate-shape merge equality |
| [Simple types](https://smithy.io/2.0/spec/simple-types.html) | Mostly | Protocol-specific timestamp rules outside `httpJson` |
| [Aggregate types](https://smithy.io/2.0/spec/aggregate-types.html) | Mostly | Defaults and client-optional semantics; recursively typed named collections |
| [Service types](https://smithy.io/2.0/spec/service-types.html) | Partial | Resource identifier/property binding rules; closure conflict/rename validation |
| [Mixins](https://smithy.io/2.0/spec/mixins.html) | Mostly | Nothing substantial beyond rule coverage listed below |
| [Constraint traits](https://smithy.io/2.0/spec/constraint-traits.html) | Partial | `@idRef` selector resolution; enforcement is generated Go, not model-wide |
| [Type refinement traits](https://smithy.io/2.0/spec/type-refinement-traits.html) | Partial | `@default`, `@clientOptional`, `@addedDefault`, input/output structure constraints |
| [Documentation traits](https://smithy.io/2.0/spec/documentation-traits.html) | Mostly | Effective-documentation fallback; no `@internal` filter |
| [Behavior traits](https://smithy.io/2.0/spec/behavior-traits.html) | None | No behavioral semantics are generated |
| [Resource traits](https://smithy.io/2.0/spec/resource-traits.html) | None | Traits are parsed generically; no property/reference/lifecycle binding |
| [Authentication traits](https://smithy.io/2.0/spec/authentication-traits.html) | None | No auth model, signing, or generated auth |
| [Serialization and Protocol traits](https://smithy.io/2.0/spec/protocol-traits.html) | Partial | XML bindings, `@indexed`, `@discriminated`; protocol-definition details |
| [Streaming](https://smithy.io/2.0/spec/streaming.html) | None | Data streams, event streams, and streaming traits have no semantics |
| [HTTP bindings](https://smithy.io/2.0/spec/http-bindings.html) | Mostly | Literal query strings, greedy labels, specificity routing, restricted headers |
| [Endpoint traits](https://smithy.io/2.0/spec/endpoint-traits.html) | None | No endpoint resolution or host labels |
| [Selectors](https://smithy.io/2.0/spec/selectors.html) | None | No selector parser, evaluator, or validator integration |
| [Model validation](https://smithy.io/2.0/spec/model-validation.html) | Minimal | Only explicit structural validators; no framework, suppressions, or overrides |
| [Shape closures](https://smithy.io/2.0/spec/shape-closures.html) | Partial | Service closure is implemented; `shapeClosures` metadata is not |
| [Smithy IDL](https://smithy.io/2.0/spec/idl.html) | Mostly | Dangling-doc/unknown-control warnings and service/resource body validation |
| [JSON AST](https://smithy.io/2.0/spec/json-ast.html) | Load only | No JSON AST serializer; loader is lenient about unknown properties |
| Smithy 2.1 additions | Mostly | `@indexed`/`@idx` range and contiguity validation |
| Code generation (outside the spec) | Go, OpenAPI, docs | One in-process generator set, one wire protocol, no external plugin transport |

---

## 1. The Smithy model

- [x] **Smithy overview**
  - [x] Semantic model of metadata plus a graph of shapes connected by shape
        IDs.
  - [x] Multiple model files assembled into one semantic model.
  - [x] Smithy IDL (`.smithy`) representation.
  - [x] JSON AST (`.json`) representation, load-only.
  - [ ] JSON AST output/serialization (see [JSON AST](#20-json-ast)).
- [x] **The semantic model**
  - [x] Metadata, shapes, members, traits, and source locations.
  - [x] First-class member shapes (`Foo$bar`, `List$member`, `Map$key`,
        `Map$value`, enum members).
  - [x] Deterministic iteration (sorted shape IDs, sorted trait IDs, stable
        generated output).
  - [x] Internal diagnostics recorded during assembly.
  - [x] Private prelude shapes cannot be resolved by relative name; public
        prelude shapes can.
  - [ ] General `@private` visibility semantics for user-model shapes (only
        prelude resolution consumes it).
- [x] **Model metadata**
  - [x] `metadata key = <node>` in IDL.
  - [x] `metadata` object in JSON AST.
  - [x] Quoted and unquoted metadata keys.
  - [x] Array metadata values concatenate across files.
  - [x] Equal values merge silently.
  - [x] Conflicting values produce `Model.MetadataConflict`.
  - [x] Syntactic shape IDs in metadata resolve and are marked.
  - [ ] Metadata keys declared with the `metadata` trait are not typed or
        validated; validator/suppression/severity metadata is not interpreted.
- [x] **Node values**
  - [x] null, boolean, string, number, array, and object values.
  - [x] Numbers keep their source spelling, avoiding float64 precision loss.
  - [x] Source locations on nodes and object keys.
  - [x] Unquoted IDL identifiers resolve to absolute shape IDs and are marked
        `SyntacticShapeID`.
  - [x] Unresolvable syntactic shape IDs produce `SyntacticShapeIdTarget`.
  - [x] Node equality is used for merge decisions.
  - [x] **Partial:** numbers are compared by their source spelling, so `1`
        and `1.0` are not equal for metadata/trait conflict resolution even
        though they denote the same number.
- [x] **Merging model files**
  - [x] Metadata merge rules above.
  - [x] Same-ID shapes of the same type are merged.
  - [x] Same-ID shapes of different types produce `Model.DuplicateShapeId`.
  - [x] Case-insensitive shape ID conflicts produce `Model.DuplicateShapeId`.
  - [x] Duplicate member definitions merge traits and detect conflicting
        targets.
  - [x] **Partial:** duplicate `@default`/`@enumValue` values across merged
        definitions silently keep the first value rather than reporting a
        conflict.
  - [x] Service properties and resource properties merge, with conflicting
        values reported.
  - [x] Traits from all definitions merge through the normal trait rules.
  - [x] Duplicate members with different targets are reported.
  - [x] Private-prelude shadowing: local declarations beat prelude declarations
        for forward references.
  - [x] `apply` statements are collected and applied after all definitions
        exist.
  - [x] **Partial:** duplicate aggregate shapes are merged member-by-member;
        the spec requires conflicting aggregate definitions to contain the same
        members targeting the same shapes. Adding a new member in a second
        definition is accepted rather than rejected.
  - [x] **Partial:** duplicate service definitions merge their property lists;
        the spec requires the same properties targets. Conflicting individual
        values are reported, but set equality is not enforced.
- [x] **Shapes**
  - [x] Simple shapes: `blob`, `boolean`, `document`, `string`, `byte`,
        `short`, `integer`, `long`, `float`, `double`, `bigInteger`,
        `bigDecimal`, `timestamp`, `enum`, `intEnum`.
  - [x] Aggregate shapes: `list`, `set` (legacy, rejected in 2.x models),
        `map`, `structure`, `union`.
  - [x] Service shapes: `service`, `operation`, `resource`.
  - [x] Member shapes are first-class and carry their own traits, target,
        container, documentation, and source location.
  - [x] Shape-specific properties: operation I/O and errors, service
        operations/resources/errors/version/rename, resource
        identifiers/properties/lifecycle/collection operations/resources, and
        `for` resource bindings.
  - [x] Synthetic 2.1 inline-collection shapes.
  - [ ] `@box` legacy behaviour is not modelled (the trait is parsed and
        accepted).
- [x] **Shape ID**
  - [x] Absolute IDs (`ns#Shape`) and relative IDs (`Shape`).
  - [x] Member IDs (`ns#Shape$member`).
  - [x] Shape ID parse/format helpers.
  - [x] Relative resolution order: `use` alias, local namespace, public
        prelude, then local namespace fallback for an unresolved diagnostic.
  - [x] Syntactic shape-ID resolution for node values.
  - [x] Case-insensitive conflict detection.
  - [x] Member name conflicts are checked case-insensitively for structures,
        unions, enums, and intEnums.
  - [x] Prelude-only shape names resolve in `smithy.api`.
  - [x] **Partial:** the IDL parser captures shape-ID syntax leniently in a few
        places (for example a bare namespace used as a member target, or a
        leading `#`/`$` in some positions). The semantic model still rejects
        unresolved IDs, but there is no standalone Shape ID ABNF conformance
        harness.
- [x] **Traits**
  - [x] Generic trait storage: trait ID plus resolved node value and source.
  - [x] Applying traits in shape definitions, on members, and via `apply`
        statements (single trait and braced blocks).
  - [x] Applying traits to forward-referenced shapes.
  - [x] Applying traits to members.
  - [x] Defining traits with `@trait`.
  - [x] Trait value coercion for annotation traits:
        structure/map ⇒ `{}`, list/set ⇒ `[]`, document ⇒ `null`, scalar ⇒
        `null` plus a validation event.
  - [x] Trait value shape checking: objects, arrays, maps, structures,
        unions, enums, intEnums, integer ranges, booleans, strings, numbers.
  - [x] Required members of trait structures are enforced.
  - [x] A trait definition's own `@range`, `@length`, and `@pattern`
        constraints are applied to values supplied when the trait is used.
  - [x] Duplicate trait applications: equal values are ignored; list/set
        values from repeated non-`apply` definitions concatenate.
  - [x] Conflicting scalar/object trait applications produce `Model.Conflict`.
  - [x] Unknown traits produce `Model.UnresolvedTrait`.
  - [x] Applying a non-trait shape produces `Model.NotATrait`.
  - [x] Explicit target checks for `@required`, `@input`, `@output`, `@error`,
        `@mixin`, `@enumValue`, and `@trait`.
  - [x] `apply` may replace a trait value, including traits inherited from
        mixins.
  - [x] **Partial:** `apply` of a list/set trait replaces the existing value
        instead of concatenating it as the spec's conflict-resolution rules
        require.
  - [ ] Trait selector enforcement (`@trait(selector: ...)`) is not
        implemented. Only the explicit checks listed above run.
  - [ ] `@trait(conflicts: ...)` and `@trait(structurallyExclusive: ...)` are
        parsed but not enforced.
  - [ ] Trait breaking-change rules (list/map member compatibility) are not
        checked.
  - [ ] Trait value alternatives allowed by the spec are incomplete:
        `"NaN"`/`"Infinity"` for float/double, numeric timestamps, and
        string-encoded bigInteger/bigDecimal are not accepted by the generic
        value validator.
- [x] **Defining traits**
  - [x] Trait definitions are discovered before applications are resolved.
  - [x] Trait shape constraints are checked when the trait is applied.
  - [x] Annotation traits are supported.
  - [x] Prelude trait definitions are available to models.
  - [x] References to traits (trait IDs) resolve through `use`, namespace, and
        prelude rules.
  - [x] The `trait` trait's target restriction (not operation/resource/service/
        member) is checked.
  - [ ] The `@trait` selector, conflicts, and structurally exclusive
        properties have no runtime effect.
  - [ ] Breaking change rules for trait shape evolution are not validated.
- [x] **Prelude**
  - [x] The real upstream `prelude.smithy` is embedded with `go:embed` and
        parsed by the project's own parser.
  - [x] Prelude shapes are assembled first in `smithy.api`; the prelude
        defines roughly 87 traits and primitive helper shapes.
  - [x] Only the traits with a Go constant or typed accessor have semantics;
        the rest are generic trait data.
  - [x] Public/private prelude visibility controls relative-name resolution.
  - [x] **Partial:** private prelude visibility only affects relative-name
        resolution; an absolute `use smithy.api#...` reference to a private
        prelude shape is still accepted.
  - [x] `smithy.api#Unit` is the default operation input/output.
  - [x] One deliberate prelude extension: `epoch-milliseconds` in the
        `timestampFormat` enum.
- [x] **Unit type**
  - [x] `smithy.api#Unit` exists as a structure shape.
  - [x] Missing operation input/output resolves to `Unit`.
  - [x] Inline operation I/O and explicit operation I/O may target `Unit`.
  - [x] Generated Go code always emits `types.Unit`.
  - [x] Generation uses `*types.Unit` in operation signatures.
  - [x] **Partial:** implicit `Unit` operation I/O is included in
        `ServiceClosure`; the specification says implicit `Unit` is not part
        of the service closure.
  - [ ] `@unitType` target/uniqueness rules and the restriction that `Unit`
        may only be referenced by operation I/O and union members are not
        validated.

## 2. Simple types

- [x] **blob**
  - [x] Model shape and Go `[]byte` mapping.
  - [x] Base64 in JSON bodies.
  - [x] Raw bytes when bound with `@httpPayload`.
  - [x] `@length` in generated validators counts bytes.
- [x] **boolean**
  - [x] Model shape, Go `bool`, JSON boolean, HTTP string forms.
- [x] **string**
  - [x] Model shape, Go `string`, UTF-8 JSON string.
  - [x] `@length` counts Unicode code points in generated validators.
  - [x] `@pattern` uses RE2 in generated validators.
  - [x] `@mediaType`, `@jsonName`, and `@enum` are available to codegen.
  - [ ] `@mediaType`-qualified strings are not base64-encoded for HTTP header
        bindings as the HTTP bindings spec requires.
- [x] **byte / short / integer / long**
  - [x] Model shapes and `int8`/`int16`/`int32`/`int64` Go mappings.
  - [x] Trait-value range checks for each width.
  - [x] `@range` generation for these types (including wide bounds compared in
        `int64`).
  - [x] Integer JSON numbers and HTTP string forms.
- [x] **float / double**
  - [x] Model shapes and `float32`/`float64` mappings.
  - [x] `@range` generation.
  - [x] JSON non-finite values fail through `encoding/json`.
  - [ ] Trait node values `"NaN"`, `"Infinity"`, and `"-Infinity"` are not
        accepted by the generic trait-value validator.
- [x] **bigInteger / bigDecimal**
  - [x] Model shapes and `*big.Int` / `*Decimal` mappings.
  - [x] bigInteger is emitted as a JSON number.
  - [x] bigDecimal uses the generated `types.Decimal` wire type, which
        preserves the JSON number literal instead of converting to a binary
        float. Strict input accepts a JSON number only; quoted decimals are
        accepted under `@relaxedInput`.
  - [ ] `@range` is not generated for bigInteger or bigDecimal (silently
        skipped).
  - [ ] String-encoded bigInteger/bigDecimal trait values are not accepted by
        the generic trait-value validator.
- [x] **timestamp**
  - [x] Model shape and `time.Time` mapping.
  - [x] `date-time` (RFC 3339/3339Nano) in generated JSON.
  - [x] `epoch-seconds` with exact fractional nanoseconds.
  - [x] `epoch-milliseconds` (gosmithy extension).
  - [x] `http-date` (IMF-fixdate).
  - [x] Member `@timestampFormat` overrides shape `@timestampFormat`.
  - [x] Timestamps are supported in JSON bodies, unions, payloads, labels,
        query parameters, and headers.
  - [ ] The HTTP header default is `date-time` in the generated code; the
        spec's `@httpHeader` serialization rules default timestamps to
        `http-date`.
- [x] **document**
  - [x] Model shape and Go `any` mapping.
  - [x] Raw JSON passthrough.
  - [x] `UseNumber` decoding keeps numbers as `json.Number`.
  - [x] Document members and collection elements get custom JSON codecs.
  - [x] Documents can be HTTP payloads.
- [x] **enum**
  - [x] `enum` shape with string members.
  - [x] Explicit values via `= "value"` or `@enumValue`.
  - [x] Implicit member-name values.
  - [x] Member traits and documentation.
  - [x] Validation: at least one member, string values, unique values.
  - [x] Go named string type with constants and `Values()`.
  - [x] `@enum` on a string shape is emitted as an enum-like Go type.
  - [x] Enum member docs and `@deprecated` are emitted.
  - [ ] Generated enum values are not validated for membership at runtime; Go
        permits any value of the named type.
- [x] **intEnum**
  - [x] `intEnum` shape with integer members.
  - [x] Validation: at least one member, integer values, unique values, int32
        range.
  - [x] Go named `int32` type with constants and `Values()`.
  - [x] `@enumValue`-backed values.
  - [ ] IntEnum values are not validated for membership at runtime.
- [x] **Timestamp serialization and deserialization**
  - [x] All four formats in both directions for `httpJson`.
  - [x] Exact epoch-seconds parsing/formatting without float64.
  - [x] Whole-second timestamps remain integers; fractional values are kept.
  - [ ] Only the `httpJson` protocol is implemented; no other protocol's
        timestamp defaults are available.

## 3. Aggregate types

- [x] **List**
  - [x] `member` shape, `List$member` ID, traits, and source location.
  - [x] Optionality: dense list members are always present; `@sparse` permits
        `null` entries.
  - [x] Go `[]T` (or named collection type).
  - [x] Sparse elements become pointer-like in Go.
  - [x] `@length` and `@uniqueItems` generation.
  - [x] JSON array encoding/decoding.
  - [x] Nested list/map/union support.
- [x] **Map**
  - [x] `key` and `value` shapes, member IDs, traits, source locations.
  - [x] Keys are never optional.
  - [x] **Partial:** key targets may be `string` or `enum`; the specification
        requires map keys to target `string` only.
  - [x] Values are present by default; `@sparse` permits `null` values.
  - [x] Go `map[string]V` (or named collection type).
  - [x] `@length` generation.
  - [x] JSON object encoding/decoding.
  - [x] Map values are validated; map keys are not validated.
  - [x] Invalid key targets produce `Model.InvalidMemberTarget`.
- [x] **Structure**
  - [x] Member shapes, IDs, traits, docs, and source locations.
  - [x] Members are optional by default.
  - [x] `@required` is enforced by generated `Validate()`; nil means absent.
  - [x] **Partial:** `@required` is accepted on any member (including list,
        map, and union members) rather than only structure members; it only has
        an effect on structure fields.
  - [x] Go structs with pointer/nilability rules.
  - [x] `@jsonName` JSON tags and explicit codec keys.
  - [x] Member constraint overrides target-shape constraints.
  - [x] Documentation comments and `@documentation`.
  - [x] `@deprecated` comments.
  - [ ] `@default` values are parsed and stored but not applied to types,
        serialization, or generated values.
  - [ ] `@clientOptional` is parsed but has no input/output type or validation
        effect.
  - [ ] Client-error correction and `@input` structure evolution rules are not
        enforced.
  - [ ] Required members of trait structures are enforced; required-member
        semantics for operation inputs are otherwise left to generated
        validators.
- [x] **Union**
  - [x] Membership, IDs, traits, and source locations.
  - [x] Go interface with one wrapper struct per member.
  - [x] Unit member types are representable (`*types.Unit` wrapper).
  - [x] One-member JSON object encoding/decoding; exactly one key is required.
  - [x] Union member docs and constraints on member targets.
  - [x] `@jsonName` is honored in union keys.
  - [ ] Union values are not validated by generated validators.
  - [ ] Unknown union members are rejected; the protocol's general rule that
        unknown JSON fields are ignored applies only to structures.
  - [ ] Adding union members is a model edit only; no compatibility checking is
        implemented.
  - [ ] An empty union is accepted; the specification requires a union to
        contain at least one member.
- [x] **Recursive shape definitions**
  - [x] The semantic model accepts recursive references through lazy target
        resolution, and closure/flattening use visited sets.
  - [x] Structures and unions can refer to themselves through
        pointers/interfaces in generated Go.
  - [ ] the specification's invalid-recursion rules are not
        checked: direct/transitive list or map self-recursion, all-required
        structure cycles, and non-productive union recursion all assemble
        without a diagnostic.
  - [ ] recursively typed named collections (for example a list
        whose element type is itself) are not specifically tested or diagnosed;
        Go may reject such a type during generated-module compilation.

## 4. Service types

- [x] **Service**
  - [x] `version` property.
  - [x] `operations`, `resources`, and `errors` references.
  - [x] `rename` map (shape IDs to names).
  - [x] Service properties merge across duplicate definitions under the
        model-merge rules.
  - [x] Service common errors are effective for every operation through
        `ServiceOperationErrors`.
  - [ ] The service `version` value's format and presence rules are not
        validated.
- [x] **Service operations**
  - [x] Resolve each listed operation and require `operation` shapes.
  - [x] `Model.InvalidReference` for wrong shape types.
  - [x] Operation closures include operations.
- [x] **Service resources**
  - [x] Resolve resource references recursively.
  - [x] Resources contribute identifiers, properties, lifecycle operations,
        operations, collection operations, and nested resources to the
        closure.
- [x] **Service closure**
  - [x] Transitive traversal from a service.
  - [x] Includes the service, operations, resources recursively, resource
        identifiers/properties, operation input/output/errors, member shapes,
        and member targets.
  - [x] Service errors are in the closure and are effective errors for each
        operation.
  - [x] Trait-only references are deliberately not followed.
  - [x] Deterministic, lexically sorted result.
  - [x] Shapes flattened from mixins are reachable through their members.
  - [x] **Partial:** the spec's closure conflict rules ("shape types allowed to
        conflict in a closure") are not validated.
  - [x] **Partial:** `rename` is applied by the Go generator when forming type
        and member names, but closure-level conflict detection and the full
        rename validation rules (no member/operation/service renames, no
        case-insensitive collisions, no rename of things outside the closure)
        are not implemented.
  - [x] **Partial:** implicit `Unit` operation I/O is included in the closure;
        the specification excludes implicit `Unit`.
- [x] **Operation**
  - [x] Explicit `input`/`output` targets.
  - [x] Inline `input := {...}` / `output := {...}` structures.
  - [x] `$operationInputSuffix` / `$operationOutputSuffix` control statements.
  - [x] Default `smithy.api#Unit` input/output.
  - [x] `errors` list resolves to `@error` structures.
  - [x] Operation mixins inherit errors and must use `Unit` I/O.
  - [x] Structural validation: input/output exist and are structure or `Unit`;
        errors are error structures.
  - [x] Operation input/output structures in the closure.
  - [x] Unknown operation properties are parser errors.
  - [ ] Duplicate errors, input == output, and single-operation use of
        `@input`/`@output` structures are not validated.
- [x] **Resource**
  - [x] `identifiers` map and `properties` map (Smithy 2.0).
  - [x] `create`, `read`, `update`, `delete`, `list`, and `put` lifecycle
        operations.
  - [x] `operations`, `collectionOperations`, and nested `resources`.
  - [x] Structural reference validation for all of the above.
  - [x] **Partial:** identifier targets may be `string` or `enum`; the
        specification requires resource identifiers to target `string`.
  - [x] Resource properties require Smithy 2.0 or later (IDL gate).
  - [ ] Child-resource containment rules (repeated parent identifiers, resource
        cycles, a resource bound at most once) are not validated.
  - [x] `for Resource` bindings on aggregate shapes and elided members.
  - [x] Resource mixins and property inheritance/merging.
  - [x] **Partial:** resource identifier binding rules (implicit and explicit
        bindings, member-to-identifier matching, `@resourceIdentifier`) are not
        implemented or validated.
  - [x] **Partial:** resource property binding rules (binding members to
        properties, binding properties to collection operations) are not
        implemented or validated.
  - [x] **Partial:** lifecycle-specific semantics (`put` semantics, create/
        read/update/delete/list bindings, resource trait lifecycle associations)
        are represented as references only.
- [x] **Resource Identifiers**
  - [x] Shape reference resolution and target-type checks.
  - [ ] Implicit identifier binding validation.
  - [ ] Explicit identifier binding validation.
  - [ ] Identifier binding to operation members.
- [x] **Resource Properties**
  - [x] Shape reference resolution and target-type checks.
  - [x] Properties are inherited through resource mixins.
  - [ ] Binding members to properties.
  - [ ] Binding properties to collection operations.
  - [ ] Resource property binding validation.
- [x] **Resource lifecycle operations**
  - [x] All six lifecycle slots are parsed, resolved, and included in the
        service closure.
  - [x] Wrong target types produce `Model.InvalidResource`.
  - [ ] Put/create/read/update/delete/list semantics beyond reference
        validation.
  - [ ] Resource trait associations (`createsResources`, `putsResources`,
        `readsResources`, `updatesResources`, `deletesResources`).

## 5. Mixins

- [x] **Traits and mixins**
  - [x] `@mixin` marks a shape as a mixin.
  - [x] `with [A, B]` on simple, aggregate, service, resource, and operation
        shapes.
  - [x] Transitive mixin flattening.
  - [x] Mixin shapes are retained in the model but flattened members and traits
        are copied to the concrete shape.
- [x] **Mixin local traits**
  - [x] `@mixin(localTraits: [...])` prevents the listed traits from being
        copied.
  - [x] The `@mixin` trait itself is never copied.
  - [ ] `localTraits` entries are not validated as trait definitions or
        applied traits; unresolvable entries are silently ignored.
- [x] **Adding and replacing traits on copied members**
  - [x] Mixin member traits are copied.
  - [x] Locally defined member traits take precedence.
  - [x] Member targets are copied and checked for conflicts.
  - [x] `apply` statements can add or replace traits on concrete shapes and
        inherited members.
- [x] **Mixins are an implementation detail of the model**
  - [x] Concrete shapes contain the flattened members after assembly.
  - [x] Mixin shapes themselves are excluded from generated data types.
  - [x] Mixin-only shapes are not reachable through member traversal.
- [x] **Mixins cannot be referenced other than as mixins**
  - [x] Member targets that are mixins produce `Model.InvalidMemberTarget`.
  - [x] Operation I/O targets that are mixins are rejected.
  - [x] Trait target behavior for mixins is covered by the explicit target
        checks that exist.
- [x] **Mixins MUST NOT introduce cycles**
  - [x] Direct and transitive cycles produce `Model.InvalidMixin`.
  - [x] Cycle detection is deterministic.
- [x] **Mixin members MUST NOT conflict**
  - [x] Conflicting inherited member targets produce `Model.InvalidMixin`.
  - [x] Locally defined members win over conflicting inherited members only when
        the target is consistent; otherwise a diagnostic is emitted.
  - [x] Elided members resolve against mixins and resource bindings in
        reference-implementation order.
- [x] **Member ordering**
  - [x] Inherited members are ordered deterministically after local members.
  - [x] Flattened results are compared against upstream
        `.flattened.smithy` fixtures.
- [x] **Mixins on shapes with non-member properties**
  - [x] Service operations/resources/errors/version/rename are inherited.
  - [x] Resource identifiers/properties/lifecycle operations/operations/
        collection operations/resources are inherited.
  - [x] Operation errors are inherited.
  - [x] Resource mixins with properties are rejected, per the spec.
  - [x] Operation mixins with non-`Unit` I/O are rejected, per the spec.
- [x] **Service mixins**
- [x] **Resource mixins**
- [x] **Operation mixins**

## 6. Constraint traits

- [x] **Constraint trait enforcement**
  - [x] Generated Go validators enforce `@length`, `@range`, `@pattern`, and
        `@uniqueItems`.
  - [x] Member constraints override the same constraint on the target shape.
  - [x] Nested structure and collection validation paths.
  - [x] Server-side operation input validation maps failures to HTTP 400.
  - [ ] Client-side input validation is not generated.
  - [ ] Constraint traits are not enforced by model validation itself; they are
        code-generation semantics.
  - [ ] Constraint trait target/selector applicability is not validated; for
        example `@length` on a structure is accepted and ignored.
  - [ ] Invalid bounds such as `min > max` or negative lengths are not checked.
  - [ ] Invalid regular expressions in applied `@pattern` values are silently
        ignored by the model validator; only the Go generator rejects
        RE2-incompatible patterns.
- [x] **`idRef` trait**
  - [x] Trait value is parsed and exposed by `Shape.IDRef()`.
  - [x] `failWhenMissing` is exposed.
  - [ ] The selector is not parsed or evaluated; `@idRef` has no validation
        effect.
- [x] **`length` trait**
  - [x] Parsed and exposed as min/max.
  - [x] Generated validation for strings (Unicode code points), blobs (bytes),
        lists/sets (items), and maps (entries).
  - [x] Member-level override of a target shape's `@length`.
  - [x] Trait-value validation against the trait definition.
- [x] **`pattern` trait**
  - [x] Parsed and exposed.
  - [x] Generated validation for strings.
  - [x] Duplicate patterns become one package-level `regexp.MustCompile`.
  - [x] Invalid or RE2-incompatible patterns fail generation with a DANGER
        diagnostic.
  - [ ] ECMAScript regular-expression semantics are not implemented; Go RE2 is
        the supported dialect.
  - [ ] Patterns are not checked against non-string targets by a trait selector.
- [x] **`private` trait**
  - [x] Prelude visibility: private prelude shapes are not relative-name
        targets.
  - [x] `Shape.IsPrivate()` accessor.
  - [ ] General model visibility and cross-namespace `@private` semantics.
- [x] **`range` trait**
  - [x] Parsed and exposed as min/max.
  - [x] Generated validation for byte/short/integer/long/float/double.
  - [x] Member-level override.
  - [x] Fractional bounds on integral shapes produce `Model.InvalidTraitValue`.
  - [ ] `@range` on bigInteger/bigDecimal is not generated.
- [x] **`uniqueItems` trait**
  - [x] Parsed and applied to list/set targets.
  - [x] Generated O(n²) validation using Go value equality
        (`reflect.DeepEqual`).
  - [x] Member-level override.
  - [ ] No model validator checks `@uniqueItems` target applicability or its
        conflict with `@sparse`.
  - [ ] Smithy value equality for every edge case (for example NaN) is not
        separately modelled; Go equality is the implementation.
- [x] **Value equality**
  - [x] Node equality drives metadata and trait merge decisions.
  - [x] Equal trait values are ignored.
  - [x] List/set trait values concatenate for repeated non-`apply`
        applications.
  - [ ] The spec's equality rules are not applied to `apply`-style list/set
        trait merges (they replace instead).
- [x] **`enum` trait**
  - [x] Parsed as a generic trait.
  - [x] `enum`-trait string shapes are emitted as named string types with
        constants and `Values()`.
  - [x] `name`, `documentation`, and `deprecated` fields are consumed by codegen.
  - [ ] Enum-trait value validation (uniqueness, valid member names, and so on)
        is not implemented in the model validators.

## 7. Type refinement traits

- [x] **`default` trait**
  - [x] Parsed from `@default(...)` and the member `= value` shorthand.
  - [x] Stored on the member shape and exposed by `Shape.Default()`.
  - [ ] Defaults are not applied to generated types or values.
  - [ ] Default value constraints are not enforced.
  - [ ] Default values are not serialized when a member is absent.
  - [ ] API-design and default-update compatibility rules are not checked.
- [ ] **`addedDefault` trait**
  - Parsed only; no semantics or code generation.
- [x] **`required` trait**
  - [x] Allowed only on members; wrong targets produce
        `Model.InvalidTraitTarget`.
  - [x] Generated validators treat nil as absent and report `is required`.
  - [x] Empty strings, zero numbers, and empty collections are not considered
        absent.
  - [ ] Client-error correction and evolution rules are not implemented.
- [ ] **`clientOptional` trait**
  - Parsed only; no input/output type or validation effect.
- [x] **`enumValue` trait**
  - [x] Allowed only on enum/intEnum members.
  - [x] Explicit enum values and intEnum values.
  - [x] Implicit enum member-name values are synthesized.
  - [x] Validation of string values, integer values, uniqueness, and int32
        range.
  - [x] Go constants use the enum values.
- [x] **`error` trait**
  - [x] Allowed only on structures.
  - [x] `client`/`server` fault is exposed and used for HTTP error defaults
        (400/500).
  - [x] Generated errors implement `error`, `ErrorCode()`, and `ErrorFault()`.
  - [x] Operation errors and service errors must target error structures.
  - [ ] `@retryable` and other error behavior traits have no retry semantics.
- [x] **`input` trait**
  - [x] Allowed only on structures.
  - [x] Inline operation input structures receive the trait.
  - [x] Operation input targets are validated as structures or `Unit`.
  - [ ] The specification's `@input` structure constraints (member
        requiredness and evolution rules) are not enforced.
- [x] **`output` trait**
  - [x] Allowed only on structures.
  - [x] Inline operation output structures receive the trait.
  - [x] Operation output targets are validated as structures or `Unit`.
  - [ ] The specification's `@output` structure constraints are not enforced.
- [x] **`sparse` trait**
  - [x] Parsed and exposed.
  - [x] Lists/sets use pointer-like elements for sparse values.
  - [x] Maps use pointer-like values for sparse values.
  - [x] JSON `null` entries are preserved.
  - [x] Validators skip nil sparse elements/values.
  - [x] HTTP query/header/query-params targets reject sparse collections.
  - [ ] The trait's selector/target and conflict rules (for example
        `@uniqueItems`) are not validated; `@sparse` on a structure is
        accepted and ignored.
- [x] **`mixin` trait**
  - [x] Marks a shape as a mixin.
  - [x] Version-gated to Smithy 2.0+.
  - [x] Cannot be applied to members.
  - [x] Flattening and local traits described in the mixins chapter.

## 8. Documentation traits

- [x] **`deprecated` trait**
  - [x] Go `Deprecated:` comments for shapes, members, and enum values.
  - [x] OpenAPI `deprecated: true` on operations, schemas, and parameters.
  - [x] Markdown `Deprecated` badges for shapes, members, and enum values.
- [x] **`documentation` trait**
  - [x] Go doc comments for shapes, members, and enum values.
  - [x] OpenAPI `description` on the service, operations, schemas,
        parameters, and response headers; enum value descriptions use
        `x-enum-descriptions`.
  - [x] Markdown prose for the service, operations, schemas, and members.
  - [x] `///` documentation comments are used when the trait is absent.
  - [x] **Partial:** member descriptions prefer the member's own
        documentation; inline simple targets fall back to the target's docs,
        while referenced schemas carry the target docs on the component
        instead of duplicating them on every `$ref` property.
- [x] **`examples` trait**
  - [x] Parsed and validated as a generic trait value (titles, input, output,
        error, `shapeId`, and content).
  - [x] OpenAPI parameter examples from example inputs.
  - [x] OpenAPI request body examples filtered to body/payload members.
  - [x] OpenAPI success response examples filtered to body/payload members.
  - [x] OpenAPI error examples attached to the matching error response.
  - [x] Markdown examples with JSON code blocks.
  - [ ] Examples are not validated against target constraints; only the
        generic trait shape is checked.
- [x] **`externalDocumentation` trait**
  - [x] OpenAPI `externalDocs` on the service and operations;
        `x-smithy-external-docs` on schemas.
  - [x] The upstream priority order (`Homepage`, `API Reference`, …) is used,
        with a deterministic fallback.
  - [x] Markdown links.
- [x] **`synthetic` trait**
  - [x] OpenAPI `x-smithy-synthetic: true` on generated schemas.
  - [x] Markdown "Generated by the assembler" badge.
  - [x] Still rejects user traits on synthetic shapes (model validation).
- [x] **`internal` trait**
  - [x] OpenAPI `x-smithy-internal: true` on services, operations, schemas,
        and properties.
  - [x] Markdown `Internal` badges.
  - [x] Matches upstream OpenAPI behavior: internal shapes are included and
        marked, not filtered.
  - [ ] No opt-in "public docs only" filter.
- [x] **`recommended` trait**
  - [x] OpenAPI `x-smithy-recommended: true` on members and parameters.
  - [x] Markdown `Recommended` badge.
- [x] **`sensitive` trait**
  - [x] OpenAPI `format: password` for sensitive strings.
  - [x] Markdown password type/badge.
  - [ ] No value redaction or logging behavior.
- [x] **`since` trait**
  - [x] OpenAPI `x-smithy-since` and `x-enum-since` for enum values.
  - [x] Markdown `Since …` badges for shapes, members, and enum values.
- [x] **`tags` trait**
  - [x] OpenAPI top-level tags and operation tags.
  - [x] Markdown tags and grouping.
- [x] **`title` trait**
  - [x] OpenAPI `info.title`, operation summaries, and schema titles.
  - [x] Markdown headings.
- [x] **`unstable` trait**
  - [x] OpenAPI `x-smithy-unstable` and `x-smithy-unstable-feature`.
  - [x] Markdown `Unstable (feature …)` badge.
- [x] **`unstableFeatures` trait**
  - [x] OpenAPI `x-smithy-unstable-features` on the document.
  - [x] Markdown service-level note.
  - [ ] No feature gating or model filtering.

## 9. Behavior traits

All behavior traits are accepted by the generic trait system, but no client,
server, or model behavior is generated.

- [ ] **Idempotency**
  - [ ] `idempotencyToken` trait semantics.
  - [ ] `idempotent` trait semantics (beyond parsing).
  - [ ] `longPoll` trait semantics.
  - [ ] `readonly` trait semantics (the model accessor exists, but codegen and
        HTTP methods do not consume it).
  - [ ] `retryable` trait semantics (no retries are generated).
- [ ] **Pagination**
  - [ ] `paginated` parsing/accessor exists, but no pagination client helpers,
        continuation-token handling, or pagination behavior.
- [ ] **Compression**
  - [ ] `requestCompression` trait.
  - [ ] Request compression behavior.
  - [ ] Client compression implementation.
  - [ ] Backward-compatibility rules.

## 10. Resource traits

- [ ] **`nestedProperties` trait**
- [ ] **`notProperty` trait**
- [ ] **`noReplace` trait**
- [ ] **`property` trait**
- [ ] **`references` trait**
  - Parsed and exposed by `Shape.References()`; no runtime resolution.
- [ ] **`resourceIdentifier` trait**
- [ ] **Resource lifecycle association traits**
  - [ ] `createsResources`
  - [ ] `putsResources`
  - [ ] `readsResources`
  - [ ] `updatesResources`
  - [ ] `deletesResources`
- [ ] **Related machinery**
  - [ ] JMESPath expressions in resource traits.
  - [ ] Implicit and explicit identifier mappings.
  - [ ] Runtime reference resolution.
  - [ ] Binding structures and property sides.

## 11. Authentication traits

All authentication traits are parsed generically and have no semantic effect.
The generated protocol has no signing, authentication, or authorization
handling.

- [ ] **`authDefinition` trait**
- [ ] **`httpBasicAuth` trait**
- [ ] **`httpDigestAuth` trait**
- [ ] **`httpBearerAuth` trait**
- [ ] **`httpApiKeyAuth` trait**
- [ ] **`optionalAuth` trait**
- [ ] **`auth` trait**
- [ ] Service authentication scheme selection, identity providers, or signing.
- [ ] SigV4 and any AWS authentication behavior.

## 12. Serialization and Protocol traits

- [x] **`protocolDefinition` trait**
  - [x] `gosmithy.protocols#httpJson` is defined in embedded Smithy IDL with
        `@protocolDefinition` and `@trait(selector: "service")`.
  - [x] `gosmithy.protocols#relaxedInput` is defined in the same model with
        `@trait(selector: "service, member")` and a Boolean `enabled` member
        defaulting to true. The Go generator applies member-over-service
        precedence to scalar operation input members only.
  - [x] The protocol trait assembles with the user model by default.
  - [x] `use gosmithy.protocols#httpJson` resolves without extra flags.
  - [x] Package-level `httpjson.Select` for protocol IDs; the CLI always uses
        `gosmithy.protocols#httpJson`.
  - [x] Selecting a non-`httpJson` protocol produces a DANGER diagnostic.
  - [x] A service applying a different protocol definition trait while
        `httpJson` is selected produces a DANGER diagnostic.
  - [ ] the `traits` list in `@protocolDefinition` is not enforced
        against the traits used by the service's shapes.
  - [ ] no validation of protocol trait selectors, protocol
        composition, or protocol trait compatibility.
- [ ] **`discriminated` trait**
  - Parsed only; no discriminated-union JSON encoding.
- [x] **`jsonName` trait**
  - [x] JSON struct tags use the member's `@jsonName`.
  - [x] Custom structure/union codecs use the effective JSON name.
  - [x] Two members resolving to the same JSON name produce
        `Model.InvalidMember`.
  - [x] JSON AST loading preserves the trait value.
- [x] **`mediaType` trait**
  - [x] **Partial:** honored for `@httpPayload` content types, with member
        value taking precedence over target value.
  - [x] Default payload types: `application/octet-stream` for blobs,
        `text/plain; charset=utf-8` for strings, `application/json` otherwise.
  - [ ] Not honored for JSON bodies in general.
  - [ ] No content negotiation or `Accept` handling.
  - [ ] `@mediaType`-qualified strings are not base64-encoded in HTTP headers.
- [x] **`timestampFormat` trait**
  - [x] `date-time`, `epoch-seconds`, `http-date`, and the gosmithy
        `epoch-milliseconds` extension.
  - [x] Member-over-shape resolution.
  - [x] JSON, payload, label, query, and header use.
  - [ ] Header timestamps default to `date-time` in generated code; the HTTP
        bindings spec defaults header timestamps to `http-date`.
  - [ ] Protocol-defined defaults beyond `httpJson` are not implemented.
- [x] **Indexed shapes**
  - [x] **`idx` trait**
    - [x] Minimal `smithy.protocols#idx` definition is embedded and assembled.
    - [x] 2.1 member-index shorthand applies the absolute `idx` trait.
    - [x] Elided members can carry a member index.
    - [x] A user-supplied `smithy.protocols#idx` definition wins over the
          built-in definition.
    - [x] Explicit `@idx` and shorthand with equal values merge as redundant
          trait applications.
    - [ ] no validation that an indexed structure/union indexes
          every member, starts at 1, is contiguous, or stays within 1–65535.
    - [ ] Mixin/index restrictions for `idx` are not enforced.
  - [ ] **`indexed` trait**
    - Not defined by the embedded `idx` model or the prelude; applying it
      without a user-supplied definition produces `Model.UnresolvedTrait`, and
      there is no service-level indexed validation.
- [ ] **XML bindings**
  - [ ] Structure/union XML serialization.
  - [ ] Custom XML element names.
  - [ ] XML attributes.
  - [ ] `xmlName` on structures and unions.
  - [ ] Simple-type XML serialization.
  - [ ] Wrapped/flattened lists.
  - [ ] Wrapped/flattened maps.
  - [ ] `xmlAttribute` trait.
  - [ ] `xmlFlattened` trait.
  - [ ] `xmlName` trait.
  - [ ] `xmlNamespace` trait.

## 13. Streaming

- [ ] **Data streams**
  - [ ] `@streaming` blobs have streaming semantics.
  - [ ] Stream framing and body handling.
  - [ ] `@requiresLength` trait.
- [ ] **Event streams**
  - [ ] Event stream shapes and unions.
  - [ ] Modeled errors in event streams.
  - [ ] Initial-request and initial-response messages.
  - [ ] Initial message client/server behavior.
  - [ ] Client event-stream behavior.
  - [ ] Event message serialization.
  - [ ] `eventHeader` trait.
  - [ ] `eventPayload` trait.
- [x] **Parsing only**
  - [x] `@streaming` and other streaming traits can be applied and stored
        generically.
  - [ ] A model using `@streaming` is accepted without a "not supported"
        diagnostic; a streaming blob is treated as an ordinary buffered blob.

## 14. HTTP bindings

The implementation targets the project's `gosmithy.protocols#httpJson`
protocol, not an AWS protocol. The binding index lives in
`model/httpbinding`, but it is invoked by the Go generator; the standalone
`gosmithy validate` command does not run it.

- [x] **`http` trait**
  - [x] `method`, `uri`, and `code` properties.
  - [x] `@http` is required for every operation when `httpJson` is selected;
        missing trait produces `HttpBinding.MissingHttp`.
  - [x] Method token validation (spec says the method is used literally; this
        implementation is stricter).
  - [x] URI must start with `/`.
  - [x] No empty path segments, no query string in the URI pattern, no fragments.
  - [x] Success code defaults to 200.
  - [x] Code range 100–599 is accepted and validated.
  - [x] Literal URI segments and `{label}` segments.
  - [x] Labels must span an entire segment.
  - [x] Every URI label must bind exactly one required `@httpLabel` input
        member.
  - [x] Every `@httpLabel` input member must appear in the URI.
  - [x] Duplicate labels are rejected.
  - [x] Trailing slashes are optional at routing time.
  - [x] Same-method route overlap produces `HttpBinding.RouteConflict`.
  - [ ] literal query-string components in the URI pattern are
        rejected as unsupported.
  - [ ] greedy labels (`{label+}`) are rejected as unsupported.
  - [ ] the spec's full label/literal pattern validation is not
        implemented.
  - [ ] specificity routing is not implemented; ambiguous routes
        are rejected at generation time instead of resolved by specificity.
  - [ ] the accepted code range is 100–599, while the spec's
        `@http` requirement allows 100–999.
- [x] **`httpError` trait**
  - [x] Status-code parsing and validation (100–599).
  - [x] Defaults: 400 for client errors, 500 for server errors.
  - [x] Duplicate wire codes across an operation's effective errors produce a
        DANGER diagnostic.
  - [x] Modeled errors are decoded by wire code on the client.
  - [x] Modeled errors are written with their declared status on the server.
- [x] **`httpHeader` trait**
  - [x] Scalar and list-of-scalar targets.
  - [x] Case-insensitive duplicate header names are rejected.
  - [x] List values are comma-separated on send and split/trimmed on receive.
  - [x] Bound header members are removed from the JSON body.
  - [x] Headers work for input and output.
  - [ ] restricted headers (`Host`, `Content-Length`,
        `Authorization`, and so on) are not diagnosed.
  - [ ] strings with `@mediaType` are not base64-encoded for header
        bindings.
  - [ ] headers on modeled error structures are not bound or
        serialized.
- [x] **`httpPrefixHeaders` trait**
  - [x] One map-of-strings member with a non-empty prefix.
  - [x] String keys and string values are required.
  - [x] Sparse maps are rejected.
  - [x] Prefix headers are written and decoded.
  - [x] Bound member is removed from the JSON body.
  - [ ] disambiguation rules with overlapping explicit
        `@httpHeader` names are not fully validated.
  - [ ] incoming suffix keys are lowercased, but outgoing map keys
        preserve their case; the protocol guide claims both sides lowercase.
- [x] **`httpLabel` trait**
  - [x] Only valid on operation input members; output labels are rejected.
  - [x] Label member must be `@required`.
  - [x] Label member must exist in the URI.
  - [x] Non-greedy labels accept string, boolean, numeric, and timestamp
        targets.
  - [x] Percent-encoding on the client and decoding on the server.
  - [x] Only top-level input structure members are bound.
  - [ ] greedy-label target restrictions are irrelevant because
        greedy labels are rejected.
- [x] **`httpPayload` trait**
  - [x] Exactly one payload member per structure.
  - [x] Input and output payloads.
  - [x] Blob payloads are raw bytes.
  - [x] String payloads are text.
  - [x] Document payloads use the custom JSON document decoder.
  - [x] Union payloads use generated union codecs.
  - [x] bigDecimal payloads preserve the decimal literal through `types.Decimal`.
  - [x] Timestamp payloads use the effective format.
  - [x] Other shapes use JSON.
  - [x] `@mediaType` on the member or target controls `Content-Type`.
  - [x] A payload cannot coexist with unbound body members.
  - [x] Only top-level input/output members are considered.
  - [ ] `@httpPayload` on modeled error structures is not bound or
        serialized by the generated server/client.
  - [ ] no general protocol-specific document-payload rules beyond
        this implementation's JSON choice.
- [x] **`httpQuery` trait**
  - [x] Non-empty parameter names.
  - [x] Duplicate query names are rejected.
  - [x] Only valid on operation input members.
  - [x] Scalar strings, booleans, numbers, timestamps, big numbers, enums, and
        intEnums.
  - [x] Lists become repeated query keys.
  - [x] Sparse collections are rejected.
  - [x] Values are percent-encoded/decoded by `net/url`.
  - [x] `@httpQueryParams` excludes explicitly bound query keys.
- [x] **`httpQueryParams` trait**
  - [x] Only valid on operation input members.
  - [x] Exactly one member.
  - [x] Requires a map with string keys.
  - [x] Values must be strings or string lists.
  - [x] Sparse maps and sparse list values are rejected.
  - [x] Explicit `@httpQuery` keys are excluded on both client and server.
  - [x] Only top-level input structure members are bound.
- [x] **`httpResponseCode` trait**
  - [x] Only valid on operation output members.
  - [x] Exactly one member.
  - [x] Requires an integer target.
  - [x] Overrides the success status at runtime.
  - [x] Client records the actual response status.
  - [ ] runtime clamping accepts 100–999 while `@http` code
        validation accepts 100–599.
- [ ] **`cors` trait**
  - Parsed only; no CORS headers or preflight handling.
- [ ] **`httpChecksumRequired` trait**
  - Parsed only; no checksum generation or validation.
- [x] **Serializing HTTP messages**
  - [x] Request method, URI, labels, query parameters, query-params maps,
        headers, prefix headers, payloads, and JSON bodies.
  - [x] Response status, headers, prefix headers, payloads, and JSON bodies.
  - [x] No body and no `Content-Type` when there is no payload/body.
  - [x] `Content-Type: application/json` for JSON bodies.
  - [x] Bound members are removed before body marshal and restored after
        decode.
  - [x] Unknown JSON fields are ignored for structures.
  - [x] Modeled errors use the `{code, message, ...members}` JSON envelope.
  - [x] Unknown routes (404 `NotFound`), wrong methods (405 with `Allow` and
        `MethodNotAllowed`), malformed requests (400 `BadRequest`), validation
        failures (400 `ValidationException` with `errors`), body limit (413
        `PayloadTooLarge`), and unhandled errors/panics (500
        `InternalFailure`).
  - [ ] no restricted-header validation, no content negotiation,
        no `Accept`/415 handling, and no error-structure header/payload
        bindings.
- [ ] **Event streams** (HTTP binding section)
  - No event-stream HTTP binding support.

## 15. Endpoint traits

- [ ] **`endpoint` trait**
  - Parsed only; no endpoint rules, labels, or resolution.
- [ ] **`hostLabel` trait**
  - Parsed only; no host label binding.
- [ ] Endpoint client behavior and endpoint middleware.
- [ ] Endpoint discovery, endpoint parameters, and rule evaluation.

## 16. Selectors

There is no selector parser, evaluator, or integration with validation or
codegen. Selector strings in traits such as `@trait(selector: ...)` and
`@idRef` are stored but never interpreted.

- [ ] Matching shapes with selectors
- [ ] Type selectors
- [ ] Attribute selectors
  - [ ] Attribute existence
  - [ ] Attribute comparison
  - [ ] String comparators
  - [ ] Numeric comparators
- [ ] Attributes
  - [ ] `id` attribute
  - [ ] `service` attribute
  - [ ] `trait` attribute
  - [ ] Node attribute
  - [ ] Empty attribute
  - [ ] Projection attribute and projection comparators
- [ ] Scoped attribute selectors
  - [ ] Context values
  - [ ] And-logic
  - [ ] Matching multiple values
  - [ ] Case-insensitive comparisons
- [ ] Neighbors
  - [ ] Forward undirected
  - [ ] Forward directed
  - [ ] Forward recursive
  - [ ] Reverse undirected
  - [ ] Reverse directed
  - [ ] Relationships
- [ ] Functions
  - [ ] `:test`
  - [ ] `:is`
  - [ ] `:not`
  - [ ] `:in`
  - [ ] `:root` and its isolation/evaluation rules
  - [ ] `:recursive`
  - [ ] `:topdown`
- [ ] Selector arguments and examples
- [ ] Variables and the `var` attribute
- [ ] Selector grammar
- [ ] Selector compliance tests

## 17. Model validation

- [x] **Minimal structural validation**
  - [x] Duplicate/invalid shape IDs (`Model.DuplicateShapeId`).
  - [x] Unresolved shape and member references (`Model.UnresolvedShape`).
  - [x] Illegal member targets (`Model.InvalidMemberTarget`).
  - [x] Duplicate or missing aggregate members (`Model.InvalidMember`).
  - [x] Unresolved traits (`Model.UnresolvedTrait`).
  - [x] Applying non-traits (`Model.NotATrait`).
  - [x] Invalid trait values (`Model.InvalidTraitValue`).
  - [x] Operation input/output type errors (`Model.InvalidOperationIo`).
  - [x] Operation error type errors (`Model.InvalidOperationError`).
  - [x] Service/resource reference errors (`Model.InvalidReference`).
  - [x] Resource identifier/property/operation errors (`Model.InvalidResource`).
  - [x] Mixin compatibility, cycles, and conflict errors
        (`Model.InvalidMixin`).
  - [x] Enum/intEnum value and uniqueness errors (`Model.InvalidEnum`).
  - [x] Explicit trait target checks (`Model.InvalidTraitTarget`).
  - [x] Shapes invalid for the declared Smithy version
        (`Model.InvalidShapeType`).
  - [x] Trait conflicts (`Model.Conflict`).
  - [x] Metadata conflicts (`Model.MetadataConflict`).
  - [x] Unresolvable syntactic shape IDs (`SyntacticShapeIdTarget`).
  - [x] HTTP binding errors from `model/httpbinding` (missing/invalid `@http`,
        invalid URI/label/target/payload/response code/error, duplicate
        bindings, route conflicts).
  - [x] **Partial:** the HTTP binding index runs during `generate`, not during
        `gosmithy validate`; validation-only runs do not report
        `HttpBinding.*` events.
  - [x] Severity levels NOTE/WARNING/DANGER, deterministic sorting, source
        locations, text and JSON CLI output, `-min-severity`, and exit code 1
        when any DANGER remains.
  - [x] JSON AST loader diagnostics carry JSON source locations.
- [ ] **Validators framework**
  - [ ] The full built-in validator suite (the Java implementation's ~78
        validators).
  - [ ] Custom validators.
  - [ ] `@suppress` and suppression metadata.
  - [ ] Severity overrides via metadata.
  - [ ] `EmitEachSelector`, `EmitNoneSelector`, and validator definitions.
  - [ ] `traitValidators` and the `metadata` trait.
  - [ ] Message templates and variable formatting.
  - [ ] Event-ID namespaces and model-validation metadata.
  - [ ] Validator selection by selector.
  - [ ] Suppression matching by event ID.
  - [ ] Note-only validators and suggestion severities.

## 18. Shape closures

- [x] **Service closure**
  - The service closure is implemented and documented under
    [Service types](#4-service-types).
  - [x] Transitive traversal of operations, resources, I/O, errors, members,
        and member targets.
  - [x] Deterministic ordering.
  - [x] Trait-only references excluded.
- [ ] **Shape closures**
  - [ ] `shapeClosures` metadata handling.
  - [ ] The distinction between a shape closure and a service closure as a
        reusable model feature.
  - [ ] Shape-closure conflict/rename rules beyond the service closure's
        reference checks.
  - [ ] Shape closure examples from the specification.

## 19. Smithy IDL

- [x] **Smithy IDL overview**
  - [x] Human-readable IDL input.
  - [x] Multiple files and multiple namespaces.
  - [x] Debug JSON AST printed by `parse` (not a conformance JSON AST).
- [x] **Lexical notes**
  - [x] Spaces, tabs, newlines, and commas as whitespace.
  - [x] `\r\n`, `\r`, and `\n` line endings normalized.
  - [x] Line comments (`//`) and documentation comments (`///`).
  - [x] Double-quoted string handling; Smithy string escapes and
        `\uXXXX` escapes.
  - [x] Text blocks (`"""`), indentation stripping, escaped newlines, and
        trailing-line behaviour.
  - [x] Numbers (integer, decimal, exponent, negative) with malformed-number
        rejection.
  - [x] Identifiers, `true`/`false`/`null`, shape IDs, and member IDs.
  - [x] Punctuation and control tokens used by the grammar.
  - [x] Tagged string literals.
  - [x] **Partial:** leading zeros (`01`, `-01`) are accepted by the number
        tokenizer even though the ABNF forbids them.
  - [x] **Partial:** a bare carriage return is accepted as a newline, and a
        `//` comment at EOF without a newline is accepted.
- [x] **Smithy IDL ABNF**
  - [x] The 2.0 grammar productions used by shape definitions, traits, node
        values, and `apply`.
  - [x] 2.1 grammar deltas.
  - [x] Parser reports the first syntax error with a source location.
  - [ ] Error recovery that continues parsing and reports multiple syntax
        errors.
- [x] **Comments**
  - [x] `//` line comments.
  - [x] `///` documentation comments attached to shapes, members, inline I/O,
        and trait targets.
  - [x] Documentation comments before `apply` are ignored.
  - [x] Documentation after traits is ignored, matching the specification's
        ordering rules.
  - [ ] dangling documentation comments are silently dropped; no
        warning event is emitted. A `///` immediately before the closing brace
        of an `enum`/`intEnum` is a parse error instead of being ignored.
  - [x] **Partial:** `///` is tokenized wherever it appears, not only when it
        is the first non-whitespace token on a line; the placement rules are
        enforced by how comments are collected, not by the lexer.
- [x] **Control section**
  - [x] `$version`.
  - [x] `$operationInputSuffix`.
  - [x] `$operationOutputSuffix`.
  - [x] Duplicate control statements are parser errors.
  - [x] Unknown control statements are recorded and ignored.
  - [ ] unknown control statements do not produce the
        specification's warning.
- [x] **Version statement**
  - [x] `"1.0"`, `"2.0"`, `"2.1"`, `"1"`, and `"2"` are parsed.
  - [x] `"2"` resolves to the newest supported minor version (2.1) when the
        parser supports it.
  - [x] A newer-than-supported version is rejected.
  - [x] The CLI assumes 2.0 when a file omits `$version`.
  - [x] 2.1-only syntax in a 2.0 file produces a clear parser error.
  - [x] 2.0-only constructs are gated by version.
  - [x] Version-specific semantics: sets invalid in 2.x, enums/intEnums require
        2.0, mixins require 2.0, inline I/O requires 2.0, resource properties
        require 2.0, elision requires 2.0, member default requires 2.0,
        member indexes and tagged literals require 2.1.
  - [x] **Partial:** `enum`/`intEnum` and `set` declarations are accepted by the
        parser under older version statements and left to semantic validation;
        the parser itself does not gate every version-specific shape kind.
- [x] **Metadata section**
  - [x] Quoted and unquoted metadata keys.
  - [x] All node types as metadata values.
  - [x] Metadata before the namespace.
  - [x] Metadata merging across files.
- [x] **Shape section**
  - [x] `namespace` declaration.
  - [x] `use` statements.
  - [x] `use` name conflicts.
  - [x] `use` cannot target members.
  - [x] Shape definitions and `apply` statements.
- [x] **Namespaces**
  - [x] Namespace declarations and shape IDs.
  - [x] Relative-name resolution.
  - [x] Public prelude fallback.
  - [x] Local forward references shadow prelude names.
- [x] **Referring to shapes**
  - [x] Absolute, relative, and member shape IDs.
  - [x] `use` aliases for root and member IDs.
  - [x] Relative shape ID resolution.
  - [x] Syntactic shape IDs in node values and their validation.
- [x] **Defining shapes**
  - [x] Simple shapes.
  - [x] Enum shapes, including implicit values.
  - [x] IntEnum shapes with explicit integer values.
  - [x] List shapes, including `for Resource` and mixins.
  - [x] Map shapes, including `for Resource` and mixins.
  - [x] Structure shapes, including `for Resource`, mixins, elision, and
        defaults.
  - [x] Union shapes.
  - [x] Service shapes.
  - [x] Operation shapes.
  - [x] Resource shapes.
  - [x] Mixins and `with` lists.
  - [x] Target elision (`$member`).
  - [x] Inline operation input/output structures.
  - [x] Inline collection declarations.
  - [x] Tagged literals.
  - [ ] service and resource bodies are parsed as node objects and
        only known properties are interpreted; unknown properties are ignored
        instead of producing a model-validation error (operation bodies do
        reject unknown properties at parse time).
- [x] **Simple shapes**
  - [x] All scalar shape declarations.
  - [x] `with` mixins on simple shapes.
- [x] **Enum shapes**
  - [x] Explicit values and implicit member-name values.
  - [x] Member traits and docs.
  - [x] Version gating.
- [x] **IntEnum shapes**
  - [x] Explicit integer values.
  - [x] Member traits and docs.
  - [x] Version gating.
- [x] **List shapes**
  - [x] `member` declaration with traits and target.
  - [x] `for Resource` binding.
  - [x] Mixins.
- [x] **Map shapes**
  - [x] `key` and `value` declarations with traits and targets.
  - [x] `for Resource` binding.
  - [x] Mixins.
- [x] **Structure shapes**
  - [x] Members, traits, docs, defaults, elision, and member indexes.
  - [x] `for Resource` binding.
  - [x] Mixins.
- [x] **Union shapes**
  - [x] Members, traits, docs, and member indexes.
  - [x] Mixins.
- [x] **Service shape**
  - [x] Body properties as a node object.
  - [x] Mixins.
- [x] **Operation shape**
  - [x] Explicit input/output targets.
  - [x] Inline input/output structures.
  - [x] `errors` list.
  - [x] Duplicate or unknown properties rejected at parse time.
  - [x] Mixins.
  - [x] Inline I/O mixins and `for Resource`.
- [x] **Inline input / output shapes**
  - [x] `$operationInputSuffix` / `$operationOutputSuffix`.
  - [x] Default `<Operation>Input` / `<Operation>Output` names.
  - [x] Synthetic structures added to the model with `@input`/`@output`.
  - [x] Inline structure traits, mixins, and docs.
  - [x] Missing I/O defaults to `Unit`.
- [x] **Inline collection declarations**
  - [x] `[T]` list targets.
  - [x] `{K: V}` map targets.
  - [x] Nested inline collections.
  - [x] Inline collections in structure, union, list, and map members.
  - [x] Local target resolution, `use` aliases, and cross-file grouping.
  - [x] Version gating to 2.1.
- [x] **Resource shape**
  - [x] Identifiers, properties, lifecycle operations, operations, collection
        operations, and nested resources.
  - [x] Mixins.
  - [x] Version gating for properties.
- [x] **Mixins**
  - [x] `with [A, B]` syntax.
  - [x] `@mixin(localTraits: [...])`.
  - [x] Version gating.
- [x] **Target Elision**
  - [x] `$member` syntax.
  - [x] Resolution from mixins.
  - [x] Resolution from `for Resource` bindings.
  - [x] Version gating.
- [x] **Documentation comment**
  - [x] Consecutive `///` lines joined into one documentation string.
  - [x] Attached to shapes, members, and inline I/O.
  - [x] Preserved through assembly and emitted by codegen.
  - [ ] misplaced/dangling documentation warnings are not surfaced.
- [x] **Applying traits**
  - [x] `@trait`, `@trait()`, `@trait(value)`, and structured shorthands.
  - [x] Multiple traits on one target.
  - [x] Trait values as objects, arrays, strings, numbers, booleans, null, and
        text blocks.
  - [x] Structure/map/union trait value syntax.
  - [x] Omitted trait values and annotation coercion.
  - [x] Trait values on members.
  - [x] Trait values on `apply` targets.
  - [x] Duplicate keys in a structured trait value are rejected.
- [x] **Apply statement**
  - [x] `apply Shape @trait`.
  - [x] `apply Shape { @a @b }`.
  - [x] Apply to members.
  - [x] Apply before the target is defined (forward references).
  - [x] Apply can replace traits inherited from mixins.
  - [x] Apply of list/set traits replaces rather than concatenates (see the
        trait-gap note in [The Smithy model](#1-the-smithy-model)).
- [x] **Node values**
  - [x] null, boolean, string, number, array, object.
  - [x] Unquoted identifiers as syntactic shape IDs.
  - [x] Text blocks and tagged literals as values.
- [x] **String values**
  - [x] Double-quoted string forms accepted by the tokenizer.
  - [x] Standard escapes, Unicode escapes, and text-block forms.
  - [x] Invalid escapes produce parse errors.
- [x] **Text blocks**
  - [x] Delimiter handling.
  - [x] Incidental whitespace removal.
  - [x] Significant trailing line.
  - [x] Escapes in text blocks.
  - [x] Escaped newlines join lines.
  - [ ] the grammar permits spaces between the opening `"""`
        and the newline; `formatTextBlock` currently requires the first
        character to be a newline and rejects `"""  \n...`.
  - [ ] `\uXXXX` surrogate values are written through without
        surrogate-pair validation.
- [x] **Tagged string literals**
  - [x] `#re` (newlines removed, value preserved as a string).
  - [x] `#b` (binary escapes decoded to a base64 blob string).
  - [x] `#hex` (hex digits, whitespace, and comments decoded to base64).
  - [x] `#timestamp` (RFC 3339 input, epoch-seconds number output).
  - [x] Tagged text blocks.
  - [x] Version gating to 2.1.
  - [x] Unknown tags and malformed payloads are parse errors.
  - [x] **Partial:** unknown tags are not recognized as tags, so they surface
        as ordinary `#`/identifier parse errors rather than a dedicated
        unknown-tag diagnostic.

## 20. JSON AST

- [x] **Top-level properties**
  - [x] `smithy` version string.
  - [x] `metadata` object.
  - [x] `shapes` object.
  - [x] Property order independence (including `smithy` not first).
  - [x] `.json` input by extension for `parse`, `validate`, `model`, and
        `generate`.
  - [ ] unknown top-level properties are ignored rather than
        rejected.
- [x] **AST shapes**
  - [x] `blob`, `boolean`, `document`, `string`, `byte`, `short`, `integer`,
        `long`, `float`, `double`, `bigInteger`, `bigDecimal`, `timestamp`.
  - [x] `enum`, `intEnum`, `list`, `map`, `structure`, `union`.
  - [x] `service`, `operation`, `resource`.
  - [x] `set` (loaded, then rejected by semantic validation for Smithy 2.x).
  - [x] `apply` entries.
  - [x] **Partial:** the `apply` entry's target key is not checked for absolute
        shape-ID syntax, unlike shape keys and member targets.
  - [x] `mixins` arrays.
  - [x] `traits` objects on shapes and members.
  - [x] `members` objects with `target` references and member traits.
  - [x] Service operations/resources/errors/version/rename.
  - [x] Operation input/output/errors.
  - [x] Resource identifiers/properties/lifecycle/operations/
        collectionOperations/resources.
  - [x] Absolute shape-ID requirements and shape reference objects.
  - [x] Source locations for JSON tokens, object keys, values, and shapes.
  - [x] Version checks (for example resource properties require 2.0).
  - [x] Max nesting depth and duplicate object keys are rejected.
  - [ ] unknown properties inside shape objects are ignored rather
        than rejected.
  - [ ] the loader does not require a list `member`, a map
        `key`/`value`, or a non-empty enum; those checks happen later in
        semantic validation.
  - [ ] JSON AST shape-ID strings inside trait node values are
        ordinary strings; there is no syntactic-shape-ID resolution equivalent
        to IDL's unquoted identifiers.
- [ ] **JSON AST output**
  - [ ] Serializer from the semantic model to JSON AST.
  - [ ] JSON AST conformance for `model --json` (the current output is a debug
        view, not a Smithy JSON AST).
  - [ ] Round-trip IDL ↔ JSON AST via a serializer.

---

## Smithy 2.1 additions

Smithy 2.1 is not a separate specification page in the 2.0 documentation, but
the project implements its language and semantics where they are specified in
the 2.0 pages.

- [x] **Inline collections**
  - [x] `[T]` and `{K: V}` syntax.
  - [x] `$version: "2.1"` and bare `$version: "2"` gating.
  - [x] Upstream-compatible synthetic names (`_SyntheticListOf...`,
        `_SyntheticMapOf...`).
  - [x] Grouping identical collections by namespace.
  - [x] Nesting.
  - [x] `@synthetic` on generated shapes.
  - [x] User-defined collision diagnostics.
  - [x] Traits cannot be applied to synthetic shapes.
  - [x] Reserved `_Synthetic` prefix warning for user shapes.
  - [x] Go types for synthetic collections, with collision-safe renaming.
- [x] **Member indexes**
  - [x] `1. member: Target` shorthand.
  - [x] Positive integer validation.
  - [x] Version gating.
  - [x] Absolute `smithy.protocols#idx` trait application.
  - [x] Elided members.
  - [x] Built-in minimal `idx` definition.
  - [x] User definition precedence.
  - [x] Equal explicit/shorthand values merge.
  - [ ] `idx` range (1–65535), contiguity, and
        all-members-indexed validation.
  - [ ] `@indexed` service-level validation.
  - [ ] Mixin/index interaction restrictions.
- [x] **Tagged string literals**
  - [x] `#re`, `#b`, `#hex`, and `#timestamp`.
  - [x] Version gating and error handling.
- [x] **Raw `@version: "2"` resolution**
  - [x] `$version: "2"` resolves to the newest supported minor version (2.1).

---

## Code generation, CLI, and protocol implementation

These areas are not chapters of the Smithy specification; they describe the
tool built on top of the model. They are included because "coverage" for this
project includes what the generator can emit.

### Code-generation contract

- [x] `Generator` interface and `GeneratorFunc` adapter.
- [x] `NamedGenerator` for stable runner diagnostics.
- [x] `Request` with model, service, version, target, protocol, options, and
      source files.
- [x] `Response`, `File`, and `Diagnostic` types.
- [x] `Registry` with deterministic names and duplicate registration protection.
- [x] `Runner` that calls all generators before writing anything.
- [x] Deterministic file merging.
- [x] Duplicate identical files are skipped; conflicting content or modes abort.
- [x] Rejection of absolute paths, volume names, backslashes, `..` segments, and
      paths that escape the output directory.
- [x] Atomic file writes (temp file + rename) and directory creation.
- [x] `--clean` with guards against deleting the filesystem root or a directory
      containing the working directory.
- [x] DANGER diagnostics abort before writing; NOTE/WARNING are reported.
- [x] Stable diagnostics sorting and written/skipped/diagnostic summary.
- [x] Dry-run support in the runner.
- [x] Runner API supports multiple generators.
- [x] CLI `--generator` is repeatable and accepts comma-separated names.
- [x] DANGER diagnostics from multiple generators are deduplicated when
      identical.
- [ ] The CLI has no `--dry-run` flag.
- [ ] Out-of-process/stdin-stdout plugin transport.
- [ ] `Target` is declared and populated but not used for dispatch.
- [ ] Rollback of files already written after a mid-run write failure.

### Go target

- [x] Naming: PascalCase, Go keyword/predeclared-name handling, reserved
      generated names, and trailing `_` for collisions.
- [x] Service `rename` applied to shape and member names.
- [x] Type-name, field-name, operation-name, and union-wrapper collision
      diagnostics; synthetic types get numeric suffixes.
- [x] Enum-constant collisions fall back to alternate names.
- [x] Shape/member docs and `@deprecated`.
- [x] All Smithy simple types mapped to Go (see [Simple types](#2-simple-types)).
- [x] Structures, lists, sets, maps, Unit, unions, enums, intEnums, and errors.
- [x] Error types implement `error`, `ErrorCode()`, and `ErrorFault()`.
- [x] Nullability rules: `Optional[T]` for non-required structure scalars by
      default, plain `T` for required scalars, pointers for HTTP-bound and
      payload members, nilable slices/maps/blobs/documents, nil interfaces for
      absent unions. `--useOptionals=false` restores pointer scalars.
- [x] `Optional[T]` provides `IsSet`, `IsNull`, `HasValue`, `Value`, `Set`,
      `SetNull`, and works with `omitzero` (Go 1.24).
- [x] `@sparse` pointer-like elements and values.
- [x] `@jsonName` tags and custom codec keys.
- [x] Synthetic 2.1 collections as named Go collection types.
- [x] `go:model`, `go:server`, and `go:client` emit the base module and the
      corresponding side.
- [x] `go` emits the base module; with an explicit protocol and no split
      generator it also emits the legacy combined client and server.
- [x] `httpJson` is always selected automatically for Go HTTP, OpenAPI, and
      docs output; there is no protocol flag.
- [x] `types.Unit` is always emitted.
- [x] Deterministic output, `go/format`, sorted imports.
- [x] Generated `go.mod` with no third-party dependencies; `go 1.24` when
      optionals are enabled, `go 1.22` otherwise.
- [ ] `@default` and `@clientOptional` semantics.
- [ ] Input/output-specific Go type variants.
- [ ] Service/resource client abstractions and pagination helpers.
- [ ] Recursively typed named collections are not specifically handled.

### Generated validation

- [x] `Validate()` on every structure, named list, named set, and named map.
- [x] Nil receivers are valid.
- [x] `@required` uses nil as absence for pointers/nilables; required scalars
      are plain values and `Optional` presence is checked with `HasValue`;
      empty values are not absent.
- [x] `@length` for strings (code points), blobs (bytes), and collections.
- [x] `@range` for integers and floats; wide bounds do not overflow.
- [x] `@pattern` with Go RE2 and clear generation errors.
- [x] `@uniqueItems` for list/set using Go value equality.
- [x] Nested validation with paths such as `a.b`, `tags[0]`, and `index[key]`.
- [x] Member constraints override target-shape constraints.
- [x] Sparse nil values are skipped.
- [ ] Union value validation.
- [ ] Map-key validation.
- [ ] `@range` on bigInteger/bigDecimal.
- [ ] `@default`/`@clientOptional` validation effects.
- [ ] Enum/intEnum membership validation.
- [ ] Cycle detection or recursion-depth guards (callers must not build cyclic
      values).
- [ ] Client-side input validation; the server validates decoded input.
- [ ] Server-side output validation.

### OpenAPI generator

- [x] `openapi:descriptor` (root `openapi.json`) and `openapi:index`
      (`docs/index.html` plus `docs/openapi.json`) options.
- [x] OpenAPI 3.0.3 JSON document.
- [x] Uses `gosmithy.protocols#httpJson` unconditionally. The HTTP binding
      index is the source for paths, parameters, bodies, and errors.
- [x] `info` from service name/title, `@title`, service version, and
      `@documentation`.
- [x] Paths from `@http(method, uri, code)` via `model/httpbinding`.
- [x] Path, query, query-params, and header parameters.
- [x] Request bodies from unbound members and `@httpPayload`.
- [x] Success responses from unbound output members and `@httpPayload`.
- [x] Modeled errors keyed by `@httpError`, including multiple errors sharing a
      status (`oneOf`).
- [x] Response headers from `@httpHeader`.
- [x] Component schemas for structures, unions, lists, sets, maps, enums, and
      intEnums; simple shapes are inlined.
- [x] `$ref` component references, including recursive schemas.
- [x] Constraints: `@length`, `@range`, `@pattern`, `@uniqueItems`, and
      `@sparse`.
- [x] `@jsonName`.
- [x] Timestamp formats (`date-time`, `epoch-seconds`, `epoch-milliseconds`,
      `http-date`).
- [x] `@mediaType` content types for payloads.
- [x] Documentation traits listed above.
- [x] Warning diagnostics for `@httpPrefixHeaders` and dynamic
      `@httpResponseCode`.
- [x] Deterministic output and golden tests.
- [ ] YAML output.
- [ ] OpenAPI 3.1 output.
- [ ] Security schemes, auth, servers, and AWS extensions.
- [ ] `@httpQueryParams` has a non-standard object-parameter representation.
- [ ] OpenAPI extension/mapper plugin system.

### Documentation generator

- [x] `docs:markdown` option registered with the CLI.
- [x] `docs/README.md` generated Markdown.
- [x] `openapi:index` provides the docs-local `openapi.json` and Redoc HTML.
- [x] Overview, operations table, operation details, parameters, bodies,
      responses, headers, schemas, constraints, and examples.
- [x] Documentation-trait badges and links.
- [x] Deterministic output and golden Markdown tests.
- [x] CLI can run Go, OpenAPI, and docs in one invocation.
- [ ] Redoc is loaded from a CDN; the HTML is client-rendered and requires the
      docs directory to be served over HTTP.
- [ ] No offline/vendored Redoc bundle, no pre-rendered HTML, and no full static
      site generator.
- [ ] Hand-written guides and tutorials are out of scope.

### `httpJson` protocol

- [x] Embedded `gosmithy.protocols#httpJson` definition.
- [x] HTTP binding index and validation.
- [x] Generated `service.go`, `client.go`, `server.go`, and `protocol.go`.
- [x] Client options: `WithHTTPClient`, `WithBasePath`, `WithHeader`, per-call
      options.
- [x] Server options: `WithMaxBodyBytes` (10 MiB default), `WithErrorLogger`,
      `WithServerBasePath`.
- [x] All HTTP bindings and JSON value encodings listed in
      [HTTP bindings](#14-http-bindings).
- [x] Modeled errors, unknown errors, validation errors, 404/405/400/413/500
      envelopes, body limits, panic recovery.
- [x] Empty operations and Unit I/O.
- [x] `httptest` round trips and hand-written wire-contract tests.
- [ ] Streaming, event streams, XML, CBOR, multipart, compression, checksums,
      content negotiation, AWS compatibility, auth/signing, retries, endpoint
      resolution, and pagination.
- [ ] Error-structure HTTP header/payload bindings.
- [ ] Restricted HTTP header validation.
- [ ] HTTP header default timestamp format (`http-date`).

### CLI

- [x] `parse`, `validate`, `model`, `generate`, `version`, and `help`.
- [x] `.smithy` and `.json` input by extension.
- [x] `parse` debug JSON with source locations.
- [x] `validate` text and `--json` output, `-min-severity`, DANGER exit code.
- [x] `model --json` deterministic debug view; `-prelude=false` and
      `-protocols=false`.
- [x] Built-in protocol definitions assembled by default.
- [x] `generate --service/--generator/--module/--out/--clean/--list-generators/
      --opt`.
- [x] Repeatable/comma-separated `--generator`; `--module` is required when
      any Go option (`go:model`, `go:server`, or `go:client`) is selected.
- [x] Generator namespaces and options: `go:model`, `go:server`, `go:client`,
      `openapi:descriptor`, `openapi:index`, `docs:markdown`.
- [x] A bare namespace expands to all of its options.
- [x] Omitting `--generator` selects every namespace and option.
- [x] No protocol-selection flag; the CLI always uses
      `gosmithy.protocols#httpJson`.
- [x] Process exit codes and stderr for fatal/usage errors.
- [ ] A config-file interface.
- [ ] `--dry-run`.

---

## Verification evidence

These are the suites that back the claims above:

- **Parser corpus:** [idl/fixtures_test.go](idl/fixtures_test.go) runs 100
  valid fixtures and 180 syntax-invalid fixtures with expected error messages.
- **Semantic invalid fixtures:** [model/semantic_invalid_test.go](model/semantic_invalid_test.go)
  checks the 27 semantic-invalid fixtures by event ID, message, and source
  location.
- **Mixin fixtures:** [model/mixins_flatten_test.go](model/mixins_flatten_test.go)
  compares flattened models against upstream `.flattened.smithy` files.
- **JSON AST:** [model/jsonast_test.go](model/jsonast_test.go) compares 80 JSON
  AST fixtures with their IDL siblings; 81 JSON fixtures load.
- **2.1 semantics:** [model/smithy21_test.go](model/smithy21_test.go) covers
  synthetic naming, grouping, nesting, union members, local targets, user
  collisions, member indexes, built-in `idx`, user `idx` precedence, and
  upstream fixtures.
- **Model/validation:** [model/assemble_test.go](model/assemble_test.go),
  [model/semantics_test.go](model/semantics_test.go),
  [validation/validate_test.go](validation/validate_test.go), and
  [validation/validation_dangers_test.go](validation/validation_dangers_test.go).
- **Parser units:** [idl/parser_test.go](idl/parser_test.go),
  [idl/tokenizer_test.go](idl/tokenizer_test.go),
  [idl/json_ast_test.go](idl/json_ast_test.go), and the fuzz target in
  [idl/fuzz_test.go](idl/fuzz_test.go).
- **Go codegen:** [codegen/golang/generator_test.go](codegen/golang/generator_test.go)
  covers golden output, deterministic output, generated validator behavior,
  naming collisions, service renames, unsupported patterns, scalar types,
  enums/unions, errors, and constraint overrides.
- **Compile matrix:** [codegen/golang/compile_matrix_test.go](codegen/golang/compile_matrix_test.go)
  generates and `go build`s 24 model/type cases plus inline-collection cases.
- **OpenAPI:** [codegen/openapi/openapi_test.go](codegen/openapi/openapi_test.go)
  covers golden output, paths, parameters, bodies, errors, payloads, docs
  traits, examples, warnings, protocol requirements, and determinism.
- **Documentation:** [codegen/docs/docs_test.go](codegen/docs/docs_test.go)
  covers the Redoc HTML, OpenAPI copy, golden Markdown, examples, badges,
  protocol requirements, and determinism.
- **Wire protocol:** [codegen/golang/protocol_test.go](codegen/golang/protocol_test.go)
  and [codegen/golang/protocol_contract_test.go](codegen/golang/protocol_contract_test.go)
  cover golden output, route conflicts, binding codecs, client/server round
  trips, modeled/unknown errors, validation, body limits, empty operations,
  options, and hand-written wire contracts.
- **HTTP binding index:** [model/httpbinding/httpbinding_test.go](model/httpbinding/httpbinding_test.go)
  and [model/httpbinding/binding_dangers_test.go](model/httpbinding/binding_dangers_test.go).
- **Runner/registry:** [codegen/runner_test.go](codegen/runner_test.go) and
  [codegen/registry_test.go](codegen/registry_test.go).
- **CLI:** [cmd/gosmithy/main_test.go](cmd/gosmithy/main_test.go) and
  [cmd/gosmithy/generate_test.go](cmd/gosmithy/generate_test.go), including
  compilation of a generated HTTP module, OpenAPI/docs-only runs, and combined
  Go+OpenAPI+docs output.
- **Runtime/protocol definitions:** [internal/runtime/runtime_test.go](internal/runtime/runtime_test.go)
  and [protocol/httpjson/httpjson_test.go](protocol/httpjson/httpjson_test.go).
- [ ] Differential testing against the Java Smithy implementation.
- [ ] The separate Smithy HTTP protocol compliance-test suite.
- [ ] Selector, validator-framework, streaming, XML, and endpoint test suites.

## Known deviations and limitations

Several implemented behaviours intentionally or accidentally differ from the
specification, or are stricter than it. They are repeated here so they are not
missed in the tree above:

1. `apply` of a list/set trait replaces the value instead of concatenating it.
2. Duplicate aggregate/service definitions are merged member-by-member instead
   of requiring identical member/property sets.
3. HTTP header timestamps default to `date-time`, not the HTTP binding's
   `http-date`.
4. `@mediaType` strings in HTTP headers are not base64-encoded.
5. Incoming `@httpPrefixHeaders` suffixes are lowercased; outgoing keys are not.
6. Restricted HTTP headers are not diagnosed.
7. `@httpPayload` and `@httpHeader` on modeled error structures are not
   serialized or decoded.
8. Literal query strings and greedy labels are rejected rather than supported.
9. Ambiguous same-method routes are rejected instead of routed by specificity.
10. `@http` code validation accepts 100–599, while the spec allows 100–999.
11. `@streaming` is accepted but silently treated as a buffered value.
12. `@indexed`, `@idx` range/contiguity, and mixin/index restrictions are not
    validated.
13. `@default`, `@clientOptional`, and `@addedDefault` have no generated
    semantics.
14. `@idRef` selector values are not interpreted.
15. JSON AST output is not implemented; `model --json` is a debug view.
16. The JSON AST loader ignores unknown properties and does not check every
    required AST property.
17. Client input validation, server output validation, union value validation,
    and map-key validation are not generated.
18. Only Go, OpenAPI, and docs are generated; `httpJson` is the only wire
    protocol.
19. Map keys and resource identifiers may target enums; the spec requires
    strings.
20. Empty unions, `@required` on non-structure members, and `@sparse` on
    non-collection targets are accepted without diagnostics.
21. Invalid recursive shapes (list/map self-recursion, all-required structure
    cycles, non-productive unions) are not detected.
22. Implicit `Unit` operation I/O is included in the service closure.
23. Duplicate `@default`/`@enumValue` values across merged definitions keep the
    first value instead of conflicting.
24. HTTP binding errors are only produced during `generate`; `gosmithy validate`
    does not run the binding index.
25. `@indexed` is not defined by the built-in model; applying it without a user
    definition is an unresolved trait.
26. Text blocks reject spaces between the opening delimiter and the newline;
    the number tokenizer accepts leading zeros.
27. OpenAPI output is 3.0.3 JSON only; YAML, 3.1, security schemes, and server
    definitions are not generated.
28. `@httpPrefixHeaders` and dynamic `@httpResponseCode` are omitted from
    OpenAPI with warnings.
29. Generated docs are API reference plus model-derived Markdown; they are not
    a full static site and Redoc depends on a CDN at view time.
