package idl

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *File {
	t.Helper()
	f, err := ParseWithOptions("test.smithy", src, Options{Version: V2_1, DefaultVersion: V2_0})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return f
}

func TestParseShapeKindsAndSources(t *testing.T) {
	src := `$version: "2.0"
namespace example.weather

string CityId
structure GetCityInput {
  @required
  cityId: CityId
}
list Cities {
  member: CityId
}
`
	f := mustParse(t, src)
	if f.Namespace != "example.weather" {
		t.Fatalf("namespace = %q", f.Namespace)
	}
	if f.Version != V2_0 {
		t.Fatalf("version = %s", f.Version)
	}
	if len(f.Shapes) != 3 {
		t.Fatalf("shapes = %d", len(f.Shapes))
	}

	input := f.Shapes[1].(*ShapeStmt)
	if input.Kind != KindStructure || input.Name != "GetCityInput" {
		t.Fatalf("shape = %+v", input)
	}
	if input.Source.Line != 5 || input.Source.Column != 1 {
		t.Fatalf("source = %+v", input.Source)
	}
	if len(input.Members) != 1 {
		t.Fatalf("members = %d", len(input.Members))
	}
	member := input.Members[0]
	if member.Name != "cityId" || member.Target.ID != "CityId" {
		t.Fatalf("member = %+v", member)
	}
	if len(member.Traits) != 1 || member.Traits[0].Name != "required" {
		t.Fatalf("member traits = %+v", member.Traits)
	}

	list := f.Shapes[2].(*ShapeStmt)
	if list.Kind != KindList || list.Members[0].Target.ID != "CityId" {
		t.Fatalf("list = %+v", list)
	}
}

func TestDocCommentsAttach(t *testing.T) {
	src := `$version: "2.0"
namespace example

/// Foo docs
/// second line
string Foo

structure S {
  /// member docs
  a: String

  @required
  /// ignored: docs after traits
  b: String
}
`
	f := mustParse(t, src)
	foo := f.Shapes[0].(*ShapeStmt)
	if foo.Doc != "Foo docs\nsecond line" {
		t.Fatalf("shape doc = %q", foo.Doc)
	}
	s := f.Shapes[1].(*ShapeStmt)
	if s.Members[0].Doc != "member docs" {
		t.Fatalf("member doc = %q", s.Members[0].Doc)
	}
	if s.Members[1].Doc != "" {
		t.Fatalf("docs after traits should be ignored, got %q", s.Members[1].Doc)
	}
}

func TestDocCommentsBeforeApplyAreIgnored(t *testing.T) {
	src := `$version: "2.0"
namespace example

string Foo

/// ignored
apply Foo @deprecated
`
	f := mustParse(t, src)
	if len(f.Shapes) != 2 {
		t.Fatalf("shapes = %d", len(f.Shapes))
	}
	apply, ok := f.Shapes[1].(*ApplyStmt)
	if !ok {
		t.Fatalf("expected apply statement, got %T", f.Shapes[1])
	}
	if apply.Target.ID != "Foo" || len(apply.Traits) != 1 {
		t.Fatalf("apply = %+v", apply)
	}
}

func TestTextBlockValues(t *testing.T) {
	src := `$version: "2.0"
namespace example

@documentation("""
    Hello
    World
    """)
string A

@documentation("""
    Hi \
    there""")
string B

@documentation("""

    """)
string C
`
	f := mustParse(t, src)
	docs := func(i int) string {
		return f.Shapes[i].(*ShapeStmt).Traits[0].Value.Str
	}
	if got, want := docs(0), "Hello\nWorld\n"; got != want {
		t.Fatalf("A = %q, want %q", got, want)
	}
	if got, want := docs(1), "Hi there"; got != want {
		t.Fatalf("B = %q, want %q", got, want)
	}
	if got, want := docs(2), "\n"; got != want {
		t.Fatalf("C = %q, want %q", got, want)
	}
}

func TestMetadataAndControl(t *testing.T) {
	src := `$version: "2.0"
$operationInputSuffix: "In"
$operationOutputSuffix: "Out"
metadata foo = "bar"
metadata list = [1, true, null, {a: b}]
namespace example

operation GetThing {
  input := { id: String }
  output := { name: String }
}
`
	f := mustParse(t, src)
	if len(f.Controls) != 3 || len(f.Metadata) != 2 {
		t.Fatalf("controls=%d metadata=%d", len(f.Controls), len(f.Metadata))
	}
	if f.Metadata[1].Value.Kind != NodeArray || len(f.Metadata[1].Value.Arr) != 4 {
		t.Fatalf("metadata array = %+v", f.Metadata[1].Value)
	}
	obj := f.Metadata[1].Value.Arr[3]
	if obj.Kind != NodeObject {
		t.Fatalf("object = %+v", obj)
	}
	// Unquoted identifiers in node values are syntactic shape IDs.
	if id, ok := obj.Get("a"); !ok || id.Kind != NodeShapeID || !id.SyntacticShapeID {
		t.Fatalf("syntactic shape id = %+v", id)
	}

	op := f.Shapes[0].(*ShapeStmt)
	if op.Kind != KindOperation || op.Input == nil || op.Output == nil {
		t.Fatalf("operation = %+v", op)
	}
	if op.Input.Target != nil {
		t.Fatalf("expected inline input, got target %+v", op.Input.Target)
	}
	if len(op.Input.Members) != 1 || op.Input.Members[0].Name != "id" {
		t.Fatalf("inline input = %+v", op.Input)
	}
}

func TestInlineCollections(t *testing.T) {
	src := `$version: "2.1"
namespace example

structure S {
  names: [String]
  tags: {String: Integer}
  nested: {String: [String]}
}
`
	f := mustParse(t, src)
	s := f.Shapes[0].(*ShapeStmt)
	if s.Members[0].Target.InlineList == nil {
		t.Fatalf("names target = %+v", s.Members[0].Target)
	}
	if s.Members[0].Target.InlineList.Element.ID != "String" {
		t.Fatalf("names element = %+v", s.Members[0].Target.InlineList.Element)
	}
	m := s.Members[1].Target.InlineMap
	if m == nil || m.Key.ID != "String" || m.Value.ID != "Integer" {
		t.Fatalf("map target = %+v", s.Members[1].Target)
	}
	nested := s.Members[2].Target.InlineMap.Value.InlineList
	if nested == nil || nested.Element.ID != "String" {
		t.Fatalf("nested target = %+v", s.Members[2].Target)
	}
}

func TestMemberIndexes(t *testing.T) {
	src := `$version: "2.1"
namespace example

structure S {
  1. first: String
  20. second: String
}
`
	f := mustParse(t, src)
	s := f.Shapes[0].(*ShapeStmt)
	if s.Members[0].Index == nil || *s.Members[0].Index != 1 {
		t.Fatalf("first index = %v", s.Members[0].Index)
	}
	if s.Members[1].Index == nil || *s.Members[1].Index != 20 {
		t.Fatalf("second index = %v", s.Members[1].Index)
	}
}

func TestTaggedLiterals(t *testing.T) {
	src := `$version: "2.1"
namespace example

@pattern(#re "^\d+$")
string A

structure S {
  @default(#timestamp "2024-01-01T00:00:00Z")
  a: Timestamp

  @default(#hex "89504e47")
  b: Blob
}
`
	f := mustParse(t, src)
	a := f.Shapes[0].(*ShapeStmt)
	if got := a.Traits[0].Value.Str; got != `^\d+$` {
		t.Fatalf("regex = %q", got)
	}
	s := f.Shapes[1].(*ShapeStmt)
	timestamp := s.Members[0].Traits[0].Value
	if timestamp.Kind != NodeNumber || timestamp.Num != "1704067200" {
		t.Fatalf("timestamp = %+v", timestamp)
	}
	hex := s.Members[1].Traits[0].Value
	if hex.Kind != NodeString || hex.Str != "iVBORw==" {
		t.Fatalf("hex = %+v", hex)
	}
}

func TestVersions(t *testing.T) {
	t.Run("missing defaults to 2.0", func(t *testing.T) {
		f, err := Parse("test.smithy", "namespace example\nstring S\n")
		if err != nil {
			t.Fatal(err)
		}
		if f.Version != V2_0 {
			t.Fatalf("version = %s", f.Version)
		}
	})

	t.Run("major only resolves to newest supported", func(t *testing.T) {
		f, err := ParseWithOptions("test.smithy", "$version: \"2\"\nnamespace example\nstring S\n", Options{Version: V2_1})
		if err != nil {
			t.Fatal(err)
		}
		if f.Version != V2_1 {
			t.Fatalf("version = %s", f.Version)
		}
	})

	t.Run("explicit 2.1 requires support", func(t *testing.T) {
		_, err := ParseWithOptions("test.smithy", "$version: \"2.1\"\nnamespace example\n", Options{Version: V2_0})
		if err == nil || !strings.Contains(err.Error(), "Unsupported Smithy version number: 2.1") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unsupported version", func(t *testing.T) {
		_, err := Parse("test.smithy", "$version: \"3.0\"\nnamespace example\n")
		if err == nil || !strings.Contains(err.Error(), "Unsupported Smithy version number: 3.0") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestVersionGatedFeatures(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "inline collections",
			src:  "$version: \"2.0\"\nnamespace example\nstructure S { a: [String] }\n",
			want: "Inline collection syntax requires Smithy IDL version 2.1 or later",
		},
		{
			name: "inline maps",
			src:  "$version: \"2.0\"\nnamespace example\nstructure S { a: {String: String} }\n",
			want: "Inline collection syntax requires Smithy IDL version 2.1 or later",
		},
		{
			name: "member indexes",
			src:  "$version: \"2.0\"\nnamespace example\nstructure S { 1. a: String }\n",
			want: "Member index syntax requires Smithy IDL version 2.1 or later",
		},
		{
			name: "tagged literals",
			src:  "$version: \"2.0\"\nnamespace example\n@pattern(#re \"x\") string S\n",
			want: "Tagged string literals require Smithy IDL version 2.1 or later",
		},
		{
			name: "mixins",
			src:  "namespace example\nstructure A with [B] {}\n",
			want: "Mixins can only be used with Smithy version 2 or later",
		},
		{
			name: "inline io",
			src:  "namespace example\noperation O { input := {} }\n",
			want: "Inlined operation inputs and outputs can only be used with Smithy version 2 or later",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The corpus assumes Smithy's historical 1.0 default. Use it here
			// too so the version gate is exercised.
			_, err := ParseWithOptions("test.smithy", tc.src, Options{
				Version:        V2_0,
				DefaultVersion: V1_0,
			})
			if err == nil {
				t.Fatalf("expected a version-gated error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "unknown shape type",
			src:  "namespace example\nbogus Foo\n",
			want: "Unknown shape type: bogus",
		},
		{
			name: "missing namespace",
			src:  "string Foo\n",
			want: "Expected a namespace definition but found IDENTIFIER('string')",
		},
		{
			name: "unclosed string",
			src:  "namespace example\n@documentation(\")\nstring Foo\n",
			want: "Expected: '\"', but found '[EOF]'",
		},
		{
			name: "invalid escape",
			src:  "namespace example\n@documentation(\"\\'\")\nstring Foo\n",
			want: "Invalid escape found in string: `\\'`",
		},
		{
			name: "invalid number",
			src:  "metadata foo = 1.\nnamespace example\n",
			want: "Invalid number '1.': '.' must be followed by a digit",
		},
		{
			name: "duplicate member",
			src:  "$version: \"2.0\"\nnamespace example\nstructure S { a: String a: String }\n",
			want: "Duplicate member of `example#S`: 'a'",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseWithOptions("test.smithy", tc.src, Options{Version: V2_1, DefaultVersion: V2_0})
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSyntacticShapeIDs(t *testing.T) {
	f := mustParse(t, `$version: "2.0"
metadata ref = SomeShape

namespace example
`)
	if f.Metadata[0].Value.Kind != NodeShapeID || !f.Metadata[0].Value.SyntacticShapeID {
		t.Fatalf("value = %+v", f.Metadata[0].Value)
	}
	if f.Metadata[0].Value.Str != "SomeShape" {
		t.Fatalf("value = %q", f.Metadata[0].Value.Str)
	}
}
