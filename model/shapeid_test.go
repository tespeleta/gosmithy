package model_test

import (
	"testing"

	"github.com/tespeleta/gosmithy/model"
)

func TestShapeIDParseAndFormat(t *testing.T) {
	valid := []struct {
		input string
		want  model.ShapeID
	}{
		{"smithy.example#Foo", model.ShapeID{Namespace: "smithy.example", Name: "Foo"}},
		{"smithy.example#Foo$bar", model.ShapeID{Namespace: "smithy.example", Name: "Foo", Member: "bar"}},
		{"Foo", model.ShapeID{Name: "Foo"}},
		{"Foo$bar", model.ShapeID{Name: "Foo", Member: "bar"}},
		{"_ns.a_b#_Shape1$member_2", model.ShapeID{Namespace: "_ns.a_b", Name: "_Shape1", Member: "member_2"}},
	}
	for _, tc := range valid {
		got, err := model.ParseShapeID(tc.input)
		if err != nil {
			t.Errorf("ParseShapeID(%q) error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseShapeID(%q) = %#v, want %#v", tc.input, got, tc.want)
		}
		if got.String() != tc.input {
			t.Errorf("round trip %q = %q", tc.input, got.String())
		}
	}

	invalid := []string{
		"",
		"#Foo",
		"ns#",
		"ns#Foo$",
		"ns#Foo$bar$baz",
		"ns#Foo#Bar",
		"ns..a#Foo",
		"ns.#Foo",
		"ns#1Foo",
		"ns#Foo-Bar",
		"Foo$1bar",
	}
	for _, input := range invalid {
		if id, err := model.ParseShapeID(input); err == nil {
			t.Errorf("ParseShapeID(%q) = %v, want error", input, id)
		}
	}
}

func TestShapeIDHelpers(t *testing.T) {
	id := model.MustShapeID("example#Foo")
	member := id.WithMember("bar")
	if !member.IsMember() || member.MemberName() != "bar" {
		t.Fatalf("WithMember = %#v", member)
	}
	if member.WithoutMember() != id {
		t.Fatalf("WithoutMember = %s", member.WithoutMember())
	}
	if !id.EqualFold(model.MustShapeID("EXAMPLE#foo")) {
		t.Errorf("EqualFold should ignore case")
	}
	if id.EqualFold(member) {
		t.Errorf("EqualFold should not consider member IDs equal to their container")
	}
	if !(model.ShapeID{}).IsZero() || id.IsZero() {
		t.Errorf("IsZero is wrong")
	}
}
