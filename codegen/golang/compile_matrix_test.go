package golang

import (
	"testing"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

// matrixCases are the type-by-binding combinations that have produced codegen
// regressions. Each case must generate with no DANGER and produce a Go module
// that builds. Simple binding cases covered by TestProtocolRoundTrip,
// TestProtocolContracts, and the golden fixtures are intentionally omitted.
var matrixCases = []struct {
	name   string
	source string
}{
	{
		name: "query_enum_list_control",
		source: matrixModel(`@enum([
    { name: "RED", value: "red" },
    { name: "BLUE", value: "blue" }
])
string Color

list Colors {
    member: Color
}

structure In {
    @httpQuery("c")
    colors: Colors
}`, matrixOpInput),
	},
	{
		name: "header_enum_list_control",
		source: matrixModel(`@enum([
    { name: "RED", value: "red" },
    { name: "BLUE", value: "blue" }
])
string Color

list Colors {
    member: Color
}

structure In {
    @httpHeader("X-C")
    colors: Colors
}`, matrixOpInput),
	},
	{
		name: "query_timestamp_scalar_control",
		source: matrixModel(`structure In {
    @httpQuery("t")
    @timestampFormat("epoch-seconds")
    t: Timestamp
}`, matrixOpInput),
	},
	{
		name: "label_enum_control",
		source: matrixModel(`@enum([
    { name: "RED", value: "red" },
    { name: "BLUE", value: "blue" }
])
string Color

structure In {
    @required
    @httpLabel
    color: Color
}`, `@http(method: "GET", uri: "/items/{color}")
operation Op {
    input: In
}`),
	},
	{
		name: "payload_document_control",
		source: matrixModel(`structure In {
    @httpPayload
    data: Document
}`, matrixOpInput),
	},
	{
		name: "sparse_body_control",
		source: matrixModel(`@sparse
list Tags {
    member: String
}

structure In {
    tags: Tags
}`, matrixOpInput),
	},
	{
		name: "sparse_map_body_control",
		source: matrixModel(`@sparse
map Attributes {
    key: String
    value: String
}

structure In {
    attributes: Attributes
}`, matrixOpInput),
	},

	// Named string values in HTTP map bindings. These are valid Smithy; the
	// generator must convert between the named Go type and string.
	{
		name: "queryparams_enum_map",
		source: matrixModel(`@enum([
    { name: "RED", value: "red" },
    { name: "BLUE", value: "blue" }
])
string Color

map ColorMap {
    key: String
    value: Color
}

structure In {
    @httpQueryParams
    params: ColorMap
}`, matrixOpInput),
	},
	{
		name: "queryparams_enum_list_map",
		source: matrixModel(`@enum([
    { name: "RED", value: "red" },
    { name: "BLUE", value: "blue" }
])
string Color

list ColorList {
    member: Color
}

map ColorLists {
    key: String
    value: ColorList
}

structure In {
    @httpQueryParams
    params: ColorLists
}`, matrixOpInput),
	},

	// @range bounds outside the Go integer type's range. Upstream accepts
	// these models; the generated comparisons must not overflow.
	{
		name: "prefix_enum_map",
		source: matrixModel(`@enum([
    { name: "RED", value: "red" },
    { name: "BLUE", value: "blue" }
])
string Color

map ColorMap {
    key: String
    value: Color
}

structure In {
    @httpPrefixHeaders("X-P-")
    headers: ColorMap
}`, matrixOpInput),
	},
	{
		name: "range_large_integer",
		source: matrixModel(`@range(max: 10000000000)
integer Wide

structure In {
    w: Wide
}`, matrixOpInput),
	},
	{
		name: "range_large_byte",
		source: matrixModel(`@range(min: 200)
byte Wide

structure In {
    w: Wide
}`, matrixOpInput),
	},

	// Supported payload/timestamp shapes. These only need to compile here; the
	// wire contract is asserted by TestProtocolContracts.
	{
		name: "query_big_decimal_control",
		source: matrixModel(`structure In {
    @httpQuery("d")
    d: BigDecimal
}`, matrixOpInput),
	},
	{
		name: "header_big_decimal_control",
		source: matrixModel(`structure In {
    @httpHeader("X-D")
    d: BigDecimal
}`, matrixOpInput),
	},
	{
		name: "payload_big_decimal_control",
		source: matrixModel(`structure In {
    @httpPayload
    amount: BigDecimal
}`, matrixOpInput),
	},
	{
		name: "query_timestamp_list",
		source: matrixModel(`list Times {
    @timestampFormat("epoch-seconds")
    member: Timestamp
}

structure In {
    @httpQuery("t")
    times: Times
}`, matrixOpInput),
	},
}

func TestCompileInlineCollections(t *testing.T) {
	requireIntegration(t)
	m := assembleSources(t, `$version: "2.1"
namespace tdd

service S {
    version: "1.0.0"
    operations: [Op]
}

union Choice {
    names: [String]
    tags: {String: Integer}
}

structure In {
    names: [String]
    tags: {String: Integer}
    nested: {String: [String]}
    choice: Choice
}

@http(method: "POST", uri: "/op")
operation Op {
    input: In
}
`)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	requireNoGeneratorDanger(t, diags)
	dir := t.TempDir()
	writeFiles(t, dir, files)
	runGo(t, dir, "build", "./...")
}

func TestCompileInlineCollectionNameCollision(t *testing.T) {
	requireIntegration(t)
	m := assembleSources(t, `$version: "2.1"
namespace tdd

service S {
    version: "1.0.0"
    operations: [Op]
}

structure syntheticListOfString {}

structure In {
    items: [String]
    collision: syntheticListOfString
}

@http(method: "POST", uri: "/op")
operation Op {
    input: In
}
`)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	requireNoGeneratorDanger(t, diags)
	dir := t.TempDir()
	writeFiles(t, dir, files)
	runGo(t, dir, "build", "./...")
}

func TestCompileMatrix(t *testing.T) {
	requireIntegration(t)
	for _, tc := range matrixCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := assembleSources(t, tc.source)
			files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			requireNoGeneratorDanger(t, diags)
			dir := t.TempDir()
			writeFiles(t, dir, files)
			runGo(t, dir, "build", "./...")
		})
	}
}

func requireNoGeneratorDanger(t *testing.T, diags []codegen.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == validation.Danger {
			t.Fatalf("unexpected DANGER diagnostic: %s", d.String())
		}
	}
}

func matrixModel(shapes, op string) string {
	return `$version: "2.0"
namespace tdd

service S {
    version: "1.0.0"
    operations: [Op]
}

` + shapes + "\n\n" + op + "\n"
}

const (
	matrixOpInput = `@http(method: "POST", uri: "/op")
operation Op {
    input: In
}`

	matrixOpOutput = `@http(method: "POST", uri: "/op")
operation Op {
    output: Out
}`
)
