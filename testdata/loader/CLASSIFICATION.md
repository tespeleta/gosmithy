# Fixture classification

This directory contains the Smithy IDL fixture corpus from the upstream Smithy
Java repository, limited to `.smithy` files:

- `valid/` — 100 files, all of which must parse with the M1 parser.
- `invalid/` — 207 files. Each file's first line (`// ...`) is the expected
  parser or assembler error.

The upstream corpus is version-aware. M1 implements the Smithy 2.0 grammar
plus the 2.1 syntax additions (member indexes, inline collections, and tagged
string literals). Semantic assembly and validation are M2 work, so invalid
fixtures whose expected failure comes from assembly/validation are skipped by
the M1 harness.

## Counts

| Category | Count |
|---|---:|
| Valid fixtures | 100 |
| Valid fixtures using 2.1 syntax/version | 18 |
| Invalid fixtures | 207 |
| Invalid syntax fixtures checked by M1 | 180 |
| Invalid semantic fixtures deferred to M2 | 27 |

## Valid 2.1 fixtures

These files either declare `$version: "2.1"`/`$version: "2"` or use 2.1-only
syntax. All of them parse with the M1 parser.

- `valid/2.1/enums.smithy`
- `valid/2.1/inline-collections-grouping.smithy`
- `valid/2.1/inline-collections-local-target.smithy`
- `valid/2.1/inline-collections-nested-local-target.smithy`
- `valid/2.1/inline-collections-union.smithy`
- `valid/2.1/inline-collections-use.smithy`
- `valid/2.1/inline-collections.smithy`
- `valid/2.1/inline-io.smithy`
- `valid/2.1/member-indexes.smithy`
- `valid/2.1/mixins.smithy`
- `valid/2.1/resource-properties.smithy`
- `valid/2.1/simple-shapes.smithy`
- `valid/2.1/tagged-literals-textblock.smithy`
- `valid/2.1/tagged-literals.smithy`
- `valid/2/inline-collections.smithy`
- `valid/2/member-indexes.smithy`
- `valid/2/tagged-literals.smithy`
- `valid/shape-id-does-not-conflict-with-tagged-literals.smithy`

## Invalid semantic fixtures (deferred to M2)

The parser accepts these files; the expected error is produced later by
semantic assembly, shape resolution, or model validation.

- `invalid/2.1/inline-collection-unresolved-target-list.smithy`
- `invalid/2.1/inline-collection-unresolved-target-map-key.smithy`
- `invalid/2.1/inline-collection-unresolved-target-map-value.smithy`
- `invalid/elision/elide-nonexistent-id.smithy`
- `invalid/elision/elided-property-type-conflicts.smithy`
- `invalid/elision/elided-type-conflicts.smithy`
- `invalid/elision/for-referencing-non-resource.smithy`
- `invalid/enums/enum-with-bad-values.smithy`
- `invalid/enums/enum-without-member.smithy`
- `invalid/enums/idl-enums-in-v1.smithy`
- `invalid/enums/int-enum-without-member.smithy`
- `invalid/enums/intEnum-with-bad-value.smithy`
- `invalid/enums/intEnum-with-float.smithy`
- `invalid/enums/intEnum-with-long.smithy`
- `invalid/lists/list-empty-members.smithy`
- `invalid/lists/no-sets-in-v2.smithy`
- `invalid/lists/set-empty-members.smithy`
- `invalid/maps/map-empty-members.smithy`
- `invalid/metadata/space-after-node-array-keyword.smithy`
- `invalid/mixins/member-mixin-conflict-1.smithy`
- `invalid/mixins/member-mixin-conflict-2.smithy`
- `invalid/mixins/mix-types.smithy`
- `invalid/mixins/mixins-trait-in-explicit-1-0.smithy`
- `invalid/mixins/mixins-trait-in-implicit-1-0.smithy`
- `invalid/mixins/operation-mixin-with-io.smithy`
- `invalid/mixins/resource-mixin-with-properties.smithy`
- `invalid/version/idl-int-enums-in-v1.smithy`

## Error matching

`TestInvalidFixtures` reads the first line of each invalid fixture, strips the
leading `//`, and asserts that the parser error contains the expected message
after normalization:

- `[EOF]` is removed (it is platform-dependent upstream);
- the ` | Model` event suffix is removed;
- CRLF, CR, and LF are all rendered as the two characters `\n`.

This mirrors the upstream `InvalidSmithyModelLoaderRunnerTest` substring
comparison while accommodating the fact that M1 reports the first parser error
instead of emitting and recovering from validation events.

## Upstream source

`smithy-model/src/test/resources/software/amazon/smithy/model/loader/` in
[awslabs/smithy](https://github.com/awslabs/smithy), Apache-2.0. See
`../NOTICE`.
