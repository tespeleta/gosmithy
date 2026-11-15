package idl

import (
	"strings"
	"testing"
)

func TestParseJSONAST(t *testing.T) {
	src := `{
		"smithy": "2.0",
		"metadata": {"foo": 1, "tags": ["a", "b"]},
		"shapes": {
			"ns.example#Service": {
				"type": "service",
				"version": "2020-01-01",
				"operations": [{"target": "ns.example#Op"}],
				"resources": [{"target": "ns.example#Res"}],
				"errors": [{"target": "ns.example#Err"}],
				"rename": {"ns.example#Widget": "FooWidget"},
				"traits": {"smithy.api#documentation": "A service"}
			},
			"ns.example#Res": {
				"type": "resource",
				"identifiers": {"id": {"target": "smithy.api#String"}},
				"properties": {"name": {"target": "smithy.api#String"}},
				"read": {"target": "ns.example#Op"},
				"operations": [{"target": "ns.example#Op"}],
				"collectionOperations": [{"target": "ns.example#Op"}],
				"resources": [{"target": "ns.example#Res2"}]
			},
			"ns.example#Res2": {"type": "resource"},
			"ns.example#Op": {
				"type": "operation",
				"input": {"target": "ns.example#OpInput"},
				"output": {"target": "ns.example#OpOutput"},
				"errors": [{"target": "ns.example#Err"}]
			},
			"ns.example#OpInput": {"type": "structure"},
			"ns.example#OpOutput": {"type": "structure"},
			"ns.example#Err": {"type": "structure"},
			"ns.example#Struct": {
				"type": "structure",
				"mixins": [{"target": "ns.example#Mixin"}],
				"members": {
					"foo": {
						"target": "ns.example#List",
						"traits": {
							"smithy.api#required": {},
							"smithy.api#documentation": "Member docs"
						}
					}
				}
			},
			"ns.example#Mixin": {"type": "structure", "traits": {"smithy.api#mixin": {}}},
			"ns.example#List": {"type": "list", "member": {"target": "smithy.api#String"}},
			"ns.example#Map": {
				"type": "map",
				"key": {"target": "smithy.api#String"},
				"value": {"target": "smithy.api#String"}
			},
			"ns.example#Enum": {
				"type": "enum",
				"members": {"FOO": {"target": "smithy.api#Unit", "traits": {"smithy.api#enumValue": "foo"}}}
			},
			"ns.example#Struct$foo": {
				"type": "apply",
				"traits": {"smithy.api#pattern": ".*"}
			}
		}
	}`
	file, err := ParseJSONAST("model.json", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if file.Version != V2_0 {
		t.Errorf("version = %s, want 2.0", file.Version)
	}
	if file.Namespace != "" {
		t.Errorf("namespace = %q, want empty", file.Namespace)
	}
	if len(file.Metadata) != 2 {
		t.Fatalf("metadata entries = %d, want 2", len(file.Metadata))
	}
	if file.Metadata[0].Key != "foo" || file.Metadata[0].Value.Kind != NodeNumber || file.Metadata[0].Value.Num != "1" {
		t.Errorf("metadata[0] = %#v", file.Metadata[0])
	}

	shapes := make(map[string]*ShapeStmt)
	for _, stmt := range file.Shapes {
		if shape, ok := stmt.(*ShapeStmt); ok {
			shapes[shape.ID] = shape
		}
	}
	if len(shapes) != 12 {
		t.Fatalf("parsed %d shapes, want 12", len(shapes))
	}

	service := shapes["ns.example#Service"]
	if service == nil || service.Kind != KindService || service.Body == nil {
		t.Fatalf("service = %#v", service)
	}
	if got, _ := service.Body.GetString("version"); got != "2020-01-01" {
		t.Errorf("service version = %q", got)
	}
	operations, _ := service.Body.Get("operations")
	if operations.Kind != NodeArray || len(operations.Arr) != 1 || operations.Arr[0].Kind != NodeString || operations.Arr[0].Str != "ns.example#Op" {
		t.Errorf("service operations = %#v", operations)
	}
	rename, _ := service.Body.Get("rename")
	if entry, ok := rename.Get("ns.example#Widget"); !ok || entry.Str != "FooWidget" {
		t.Errorf("service rename = %#v", rename)
	}
	if len(service.Traits) != 1 || service.Traits[0].Name != "smithy.api#documentation" {
		t.Errorf("service traits = %#v", service.Traits)
	}

	resource := shapes["ns.example#Res"]
	if resource == nil || resource.Kind != KindResource || resource.Body == nil {
		t.Fatalf("resource = %#v", resource)
	}
	identifiers, _ := resource.Body.Get("identifiers")
	if entry, ok := identifiers.Get("id"); !ok || entry.Str != "smithy.api#String" {
		t.Errorf("resource identifiers = %#v", identifiers)
	}
	read, _ := resource.Body.Get("read")
	if read.Str != "ns.example#Op" {
		t.Errorf("resource read = %#v", read)
	}

	op := shapes["ns.example#Op"]
	if op == nil || op.Input == nil || op.Input.Target == nil || op.Input.Target.ID != "ns.example#OpInput" {
		t.Fatalf("op input = %#v", op)
	}
	if op.Output == nil || op.Output.Target == nil || op.Output.Target.ID != "ns.example#OpOutput" {
		t.Errorf("op output = %#v", op.Output)
	}
	if len(op.Errors) != 1 || op.Errors[0].ID != "ns.example#Err" {
		t.Errorf("op errors = %#v", op.Errors)
	}

	structShape := shapes["ns.example#Struct"]
	if structShape == nil || len(structShape.Members) != 1 {
		t.Fatalf("struct = %#v", structShape)
	}
	member := structShape.Members[0]
	if member.Name != "foo" || member.Target.ID != "ns.example#List" {
		t.Errorf("member = %#v", member)
	}
	if len(member.Traits) != 2 {
		t.Errorf("member traits = %#v", member.Traits)
	}
	if len(structShape.Mixins) != 1 || structShape.Mixins[0].ID != "ns.example#Mixin" {
		t.Errorf("mixins = %#v", structShape.Mixins)
	}

	list := shapes["ns.example#List"]
	if list == nil || len(list.Members) != 1 || list.Members[0].Name != "member" || list.Members[0].Target.ID != "smithy.api#String" {
		t.Errorf("list = %#v", list)
	}
	m := shapes["ns.example#Map"]
	if m == nil || len(m.Members) != 2 || m.Members[0].Name != "key" || m.Members[1].Name != "value" {
		t.Errorf("map = %#v", m)
	}
	enum := shapes["ns.example#Enum"]
	if enum == nil || len(enum.Members) != 1 || enum.Members[0].Name != "FOO" {
		t.Errorf("enum = %#v", enum)
	}

	var apply *ApplyStmt
	for _, stmt := range file.Shapes {
		if a, ok := stmt.(*ApplyStmt); ok {
			apply = a
		}
	}
	if apply == nil || apply.Target.ID != "ns.example#Struct$foo" || len(apply.Traits) != 1 {
		t.Fatalf("apply = %#v", apply)
	}
}

func TestParseJSONASTVersions(t *testing.T) {
	cases := map[string]Version{
		"1.0": V1_0,
		"1":   V1_0,
		"2.0": V2_0,
		"2":   V2_1,
		"2.1": V2_1,
	}
	for version, want := range cases {
		file, err := ParseJSONAST("model.json", []byte(`{"smithy": "`+version+`"}`))
		if err != nil {
			t.Fatalf("version %q: %v", version, err)
		}
		if file.Version != want {
			t.Errorf("version %q = %s, want %s", version, file.Version, want)
		}
	}
}

func TestParseJSONASTShapesBeforeVersion(t *testing.T) {
	src := `{"shapes": {"ns.example#Foo": {"type": "string"}}, "smithy": "2.0"}`
	file, err := ParseJSONAST("model.json", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if file.Version != V2_0 || len(file.Shapes) != 1 {
		t.Fatalf("file = %#v", file)
	}
}

func TestParseJSONASTLocations(t *testing.T) {
	src := "{\n  \"smithy\": \"2.0\",\n  \"shapes\": {\n    \"ns.example#Foo\": {\"type\": \"string\"}\n  }\n}"
	file, err := ParseJSONAST("model.json", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	shape := file.Shapes[0].(*ShapeStmt)
	if shape.Source.Filename != "model.json" || shape.Source.Line != 4 || shape.Source.Column != 23 {
		t.Errorf("shape source = %s", shape.Source)
	}
}

func TestParseJSONASTErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"malformed", `{"smithy": "2.0", "shapes": {`, "Syntax error"},
		{"trailing content", `{"smithy": "2.0"} {}`, "trailing content"},
		{"duplicate key", `{"smithy": "2.0", "metadata": {"a": 1, "a": 2}}`, "Duplicate object key"},
		{"not an object", `[]`, "Expected a JSON object at the document root"},
		{"missing version", `{"shapes": {}}`, "missing the `smithy` version property"},
		{"bad version", `{"smithy": "3.0"}`, "Unsupported Smithy version number: 3.0"},
		{"version not string", `{"smithy": 2.0}`, "Expected `smithy` to be a string"},
		{"shapes not object", `{"smithy": "2.0", "shapes": []}`, "Expected `shapes` to be an object"},
		{"metadata not object", `{"smithy": "2.0", "metadata": []}`, "Expected `metadata` to be an object"},
		{"missing type", `{"smithy": "2.0", "shapes": {"ns.example#Foo": {}}}`, "Missing expected member `type`"},
		{"unknown type", `{"smithy": "2.0", "shapes": {"ns.example#Foo": {"type": "widget"}}}`, "Invalid shape `type`: widget"},
		{"relative shape id", `{"smithy": "2.0", "shapes": {"Foo": {"type": "string"}}}`, "Expected an absolute shape ID"},
		{"missing member target", `{"smithy": "2.0", "shapes": {"ns.example#Foo": {"type": "structure", "members": {"bar": {}}}}}`, "Missing expected member `target`"},
		{"relative member target", `{"smithy": "2.0", "shapes": {"ns.example#Foo": {"type": "structure", "members": {"bar": {"target": "String"}}}}}`, "Expected an absolute shape ID"},
		{"missing reference target", `{"smithy": "2.0", "shapes": {"ns.example#Foo": {"type": "service", "operations": [{}]}}}`, "Missing expected member `target`"},
		{"bad traits", `{"smithy": "2.0", "shapes": {"ns.example#Foo": {"type": "string", "traits": []}}}`, "Expected `traits` to be an object"},
		{"resource properties in 1.0", `{"smithy": "1.0", "shapes": {"ns.example#Foo": {"type": "resource", "properties": {"a": {"target": "smithy.api#String"}}}}}`, "Resource properties can only be used with Smithy version 2 or later."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseJSONAST("model.json", []byte(tc.src))
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestParseJSONASTTypeNotFirst(t *testing.T) {
	src := `{"smithy": "2.0", "shapes": {"ns.example#Foo": {
		"members": {"bar": {"target": "smithy.api#String"}},
		"traits": {"smithy.api#documentation": "hi"},
		"type": "structure"
	}}}`
	file, err := ParseJSONAST("model.json", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	shape := file.Shapes[0].(*ShapeStmt)
	if shape.Kind != KindStructure || len(shape.Members) != 1 {
		t.Fatalf("shape = %#v", shape)
	}
	if len(shape.Traits) != 1 || shape.Traits[0].Name != "smithy.api#documentation" {
		t.Errorf("traits = %#v", shape.Traits)
	}
}

func TestParseJSONASTEmptyDocumentation(t *testing.T) {
	src := `{"smithy": "2.0", "shapes": {"ns.example#Foo": {
		"type": "string",
		"traits": {"smithy.api#documentation": ""}
	}}}`
	file, err := ParseJSONAST("model.json", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	shape := file.Shapes[0].(*ShapeStmt)
	if len(shape.Traits) != 1 || shape.Traits[0].Value.Kind != NodeString || shape.Traits[0].Value.Str != "" {
		t.Errorf("traits = %#v", shape.Traits)
	}
}
