package golang

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

// matrixCases are the type-by-binding combinations that have produced codegen
// regressions. Each case is generated and then checked statically: field types,
// JSON tags, and the client/server binding expressions. Compilation is covered
// once, by TestCompileKitchenSink, instead of by one module per case.
var matrixCases = []struct {
	name   string
	source string
	assert []shapeAssertion
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
		assert: []shapeAssertion{
			has("types/types.go", "type Colors []Color", "Colors Colors `json:\"colors,omitempty\"`"),
			has("types/enums.go", "type Color string", `ColorRed  Color = "red"`),
			has("client.go", "for _, elem := range input.Colors {", `query.Add("c", string(elem))`),
			has("server.go", "input.Colors = values1", "parsed2 := types.Color(value)"),
		},
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
		assert: []shapeAssertion{
			has("types/types.go", "type Colors []Color"),
			has("client.go",
				"values := make([]string, len(input.Colors))",
				"values[i] = string(elem)",
				`request.Header.Set("X-C", strings.Join(values, ", "))`),
			has("server.go", "values1 := make(types.Colors, 0, len(parts))", "parsed2 := types.Color(part)"),
		},
	},
	{
		name: "query_timestamp_scalar_control",
		source: matrixModel(`structure In {
    @httpQuery("t")
    @timestampFormat("epoch-seconds")
    t: Timestamp
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "T *time.Time `json:\"t,omitempty\"`"),
			has("client.go", `query.Add("t", formatTimestamp(*input.T, "epoch-seconds"))`),
			has("server.go", `smithy.ParseTimestamp(values[0], "epoch-seconds")`),
		},
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
		assert: []shapeAssertion{
			has("types/types.go", "Color *Color `json:\"color,omitempty\"`"),
			has("client.go", `path += "/items"`, `url.PathEscape(string(*input.Color))`),
			has("server.go", `labelValue1, ok2 := labels["color"]`, "parsed3 := types.Color(labelValue1)"),
		},
	},
	{
		name: "payload_document_control",
		source: matrixModel(`structure In {
    @httpPayload
    data: Document
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "Data any `json:\"data,omitempty\"`"),
			has("client.go", "payload, err := json.Marshal(input.Data)"),
			has("server.go", "value, err := smithy.UnmarshalDocument(body)", "input.Data = value"),
		},
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
		assert: []shapeAssertion{
			has("types/types.go", "type Tags []*string", "Tags Tags `json:\"tags,omitempty\"`"),
			has("client.go", "bodyCopy := *input", "payload, err := json.Marshal(&bodyCopy)"),
		},
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
		assert: []shapeAssertion{
			has("types/types.go", "type Attributes map[string]*string", "Attributes Attributes `json:\"attributes,omitempty\"`"),
			has("client.go", "bodyCopy := *input", "payload, err := json.Marshal(&bodyCopy)"),
		},
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
		assert: []shapeAssertion{
			has("types/types.go", "type ColorMap map[string]Color"),
			has("client.go", "for key, value := range input.Params {", `query.Add(key, string(value))`),
			has("server.go", "input.Params = make(types.ColorMap, len(query))", "input.Params[key] = types.Color(values[0])"),
		},
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
		assert: []shapeAssertion{
			has("types/types.go", "type ColorList []Color", "type ColorLists map[string]ColorList"),
			has("client.go", "for key, values := range input.Params {", `query.Add(key, string(value))`),
			has("server.go", "input.Params = make(types.ColorLists, len(query))", "parsedValues1 := make(types.ColorList, 0, len(values))"),
		},
	},

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
		assert: []shapeAssertion{
			has("types/types.go", "type ColorMap map[string]Color"),
			has("client.go", `request.Header.Set("X-P-"+key, string(value))`),
			has("server.go", `strings.EqualFold(key[:len("X-P-")], "X-P-")`, "types.Color(values[0])"),
		},
	},
	// @range bounds outside the Go integer type's range. Upstream accepts
	// these models; the generated comparisons must not overflow.
	{
		name: "range_large_integer",
		source: matrixModel(`@range(max: 10000000000)
integer Wide

structure In {
    w: Wide
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "W *int32 `json:\"w,omitempty\"`"),
			has("types/validate.go", "if int64(*v.W) > 10000000000 {"),
			has("types/validate.go", "must be less than or equal to 1e+10"),
		},
	},
	{
		name: "range_large_byte",
		source: matrixModel(`@range(min: 200)
byte Wide

structure In {
    w: Wide
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "W *int8 `json:\"w,omitempty\"`"),
			has("types/validate.go", "if int64(*v.W) < 200 {"),
			has("types/validate.go", "must be greater than or equal to 200"),
		},
	},
	{
		name: "range_big_integer",
		source: matrixModel(`@range(min: -123456789012345678901234567890, max: 123456789012345678901234567890)
bigInteger Huge

structure In {
    h: Huge
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "H *big.Int `json:\"h,omitempty\"`"),
			has("types/validate.go", `if bigIntLess(v.H, "-123456789012345678901234567890") {`),
			has("types/validate.go", `if bigIntGreater(v.H, "123456789012345678901234567890") {`),
			has("types/validate.go", "must be greater than or equal to -123456789012345678901234567890"),
			has("types/validate.go", "must be less than or equal to 123456789012345678901234567890"),
		},
	},
	{
		name: "range_big_decimal",
		source: matrixModel(`@range(min: 0.000000000000000000000000000001, max: 100.500000000000000000000000000001)
bigDecimal Precise

structure In {
    p: Precise
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "P *Decimal `json:\"p,omitempty\"`"),
			has("types/validate.go", `if decimalLess(*v.P, Decimal("0.000000000000000000000000000001")) {`),
			has("types/validate.go", `if decimalGreater(*v.P, Decimal("100.500000000000000000000000000001")) {`),
		},
	},

	// Supported payload/timestamp shapes. These only need to compile here; the
	// wire contract is asserted by TestProtocolContracts.
	{
		name: "query_big_decimal_control",
		source: matrixModel(`structure In {
    @httpQuery("d")
    d: BigDecimal
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "D *Decimal `json:\"d,omitempty\"`"),
			has("client.go", `query.Add("d", (*input.D).String())`),
			has("server.go", "input.D = &parsed1"),
		},
	},
	{
		name: "header_big_decimal_control",
		source: matrixModel(`structure In {
    @httpHeader("X-D")
    d: BigDecimal
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "D *Decimal `json:\"d,omitempty\"`"),
			has("client.go", `request.Header.Set("X-D", (*input.D).String())`),
		},
	},
	{
		name: "payload_big_decimal_control",
		source: matrixModel(`structure In {
    @httpPayload
    amount: BigDecimal
}`, matrixOpInput),
		assert: []shapeAssertion{
			has("types/types.go", "Amount *Decimal `json:\"amount,omitempty\"`"),
			has("client.go", "payload, err := json.Marshal(*input.Amount)"),
			has("server.go", "json.Unmarshal(body, &input.Amount)"),
		},
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
		assert: []shapeAssertion{
			has("types/types.go", "type Times []time.Time"),
			has("client.go", `query.Add("t", formatTimestamp(elem, "epoch-seconds"))`),
			has("server.go", "values1 := make(types.Times, 0, len(values))", `smithy.ParseTimestamp(value, "epoch-seconds")`),
		},
	},
}

func TestGeneratedBindingShapes(t *testing.T) {
	for _, tc := range matrixCases {
		t.Run(tc.name, func(t *testing.T) {
			files, diags, err := runProtocolGenerator(t, assembleSources(t, tc.source), model.MustShapeID("tdd#S"), "example.com/contract")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			requireNoGeneratorDanger(t, diags)
			assertGeneratedContent(t, files, tc.assert)
		})
	}
}

// TestGeneratedInlineCollections covers synthetic collection names, the
// collision between a synthetic name and a modeled structure, and inline
// collections inside a union.
func TestGeneratedInlineCollections(t *testing.T) {
	cases := []struct {
		name   string
		source string
		assert []shapeAssertion
	}{
		{
			name: "inline_collections",
			source: `$version: "2.1"
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
`,
			assert: []shapeAssertion{
				has("types/types.go",
					"type SyntheticListOfString []string",
					"type SyntheticMapOfStringToInteger map[string]int32",
					"type SyntheticMapOfStringToSyntheticListOfString map[string]SyntheticListOfString",
					"Names SyntheticListOfString `json:\"names,omitempty\"`",
					"Nested SyntheticMapOfStringToSyntheticListOfString `json:\"nested,omitempty\"`"),
				has("types/unions.go",
					"type ChoiceNames struct", "Value *SyntheticListOfString",
					"type ChoiceTags struct", "Value *SyntheticMapOfStringToInteger"),
			},
		},
		{
			name: "name_collision",
			source: `$version: "2.1"
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
`,
			assert: []shapeAssertion{
				has("types/types.go",
					"type SyntheticListOfString2 []string",
					"type SyntheticListOfString struct{}",
					"Items SyntheticListOfString2 `json:\"items,omitempty\"`",
					"Collision *SyntheticListOfString `json:\"collision,omitempty\"`"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files, diags, err := runProtocolGenerator(t, assembleSources(t, tc.source), model.MustShapeID("tdd#S"), "example.com/contract")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			requireNoGeneratorDanger(t, diags)
			assertGeneratedContent(t, files, tc.assert)
		})
	}
}

// TestCompileKitchenSink keeps the compile-error signal for the binding
// combinations above in a single generated module. It covers body, query,
// header, payload, map, sparse, range, and inline-collection shapes across
// several operations.
func TestCompileKitchenSink(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	m := assembleSources(t, kitchenSinkModel)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/kitchen")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	requireNoGeneratorDanger(t, diags)
	dir := t.TempDir()
	writeFiles(t, dir, files)
	runGo(t, dir, "build", "./...")
	runGo(t, dir, "vet", "./...")
}

// kitchenSinkModel is compiled as-is, so it intentionally mixes shapes that
// could not share a namespace in the per-case static tests.
const kitchenSinkModel = `$version: "2.1"
namespace tdd

use gosmithy.protocols#httpJson

@httpJson
service S {
    operations: [
        QueryOp
        HeaderOp
        EnumLabelOp
        PayloadDocumentOp
        PayloadDecimalOp
        SparseOp
        RangeOp
        InlineOp
        TimestampListOp
    ]
}

@enum([
    { name: "RED", value: "red" },
    { name: "BLUE", value: "blue" }
])
string Color

list Colors {
    member: Color
}

map ColorMap {
    key: String
    value: Color
}

list ColorList {
    member: Color
}

map ColorLists {
    key: String
    value: ColorList
}

@sparse
list SparseTags {
    member: String
}

@sparse
map SparseAttributes {
    key: String
    value: String
}

@range(max: 10000000000)
integer Wide

@range(min: 200)
byte Narrow

@range(min: -123456789012345678901234567890, max: 123456789012345678901234567890)
bigInteger Huge

@range(min: 0.000000000000000000000000000001, max: 100.500000000000000000000000000001)
bigDecimal Precise

list Times {
    @timestampFormat("epoch-seconds")
    member: Timestamp
}

structure QueryIn {
    @httpQuery("c")
    colors: Colors

    @httpQuery("t")
    @timestampFormat("epoch-seconds")
    t: Timestamp

    @httpQueryParams
    params: ColorMap
}

@http(method: "GET", uri: "/query")
operation QueryOp {
    input: QueryIn
}

structure HeaderIn {
    @httpHeader("X-C")
    colors: Colors

    @httpPrefixHeaders("X-P-")
    headers: ColorMap

    @httpHeader("X-D")
    d: BigDecimal
}

@http(method: "GET", uri: "/header")
operation HeaderOp {
    input: HeaderIn
}

structure LabelIn {
    @required
    @httpLabel
    color: Color
}

@http(method: "GET", uri: "/label/{color}")
operation EnumLabelOp {
    input: LabelIn
}

structure PayloadDocumentIn {
    @httpPayload
    data: Document
}

@http(method: "POST", uri: "/document")
operation PayloadDocumentOp {
    input: PayloadDocumentIn
}

structure PayloadDecimalIn {
    @httpPayload
    amount: BigDecimal
}

@http(method: "POST", uri: "/decimal")
operation PayloadDecimalOp {
    input: PayloadDecimalIn
}

structure SparseIn {
    tags: SparseTags
    attributes: SparseAttributes
}

@http(method: "POST", uri: "/sparse")
operation SparseOp {
    input: SparseIn
}

structure RangeIn {
    wide: Wide
    narrow: Narrow
    huge: Huge
    precise: Precise
}

@http(method: "POST", uri: "/range")
operation RangeOp {
    input: RangeIn
}

structure syntheticListOfString {}

union InlineChoice {
    names: [String]
    tags: {String: Integer}
}

structure InlineIn {
    items: [String]
    collision: syntheticListOfString
    tags: {String: Integer}
    nested: {String: [String]}
    choice: InlineChoice
}

@http(method: "POST", uri: "/inline")
operation InlineOp {
    input: InlineIn
}

structure TimestampListIn {
    @httpQuery("t")
    times: Times
}

@http(method: "GET", uri: "/times")
operation TimestampListOp {
    input: TimestampListIn
}
`

type shapeAssertion struct {
	path    string
	want    []string
	notWant []string
}

func has(path string, want ...string) shapeAssertion {
	return shapeAssertion{path: path, want: want}
}

func lacks(path string, notWant ...string) shapeAssertion {
	return shapeAssertion{path: path, notWant: notWant}
}

var generatedWhitespace = regexp.MustCompile(`\s+`)

// assertGeneratedContent checks normalized substrings of generated files, so
// gofmt alignment does not make the assertions brittle.
func assertGeneratedContent(t *testing.T, files []codegen.File, assertions []shapeAssertion) {
	t.Helper()
	for _, a := range assertions {
		content := generatedWhitespace.ReplaceAllString(generatedFileContent(t, files, a.path), " ")
		for _, want := range a.want {
			if !strings.Contains(content, generatedWhitespace.ReplaceAllString(want, " ")) {
				t.Errorf("%s does not contain %q:\n%s", a.path, want, content)
			}
		}
		for _, notWant := range a.notWant {
			if strings.Contains(content, generatedWhitespace.ReplaceAllString(notWant, " ")) {
				t.Errorf("%s unexpectedly contains %q:\n%s", a.path, notWant, content)
			}
		}
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

const matrixOpInput = `@http(method: "POST", uri: "/op")
operation Op {
    input: In
}`
