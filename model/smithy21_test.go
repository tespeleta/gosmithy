package model_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

func requireNoDangerFor(t *testing.T, m *model.Model) {
	t.Helper()
	for _, e := range validation.Validate(m) {
		if e.Severity == validation.Danger {
			t.Errorf("unexpected danger event: %s [%s]", e.String(), e.ID)
		}
	}
}

func requireShape(t *testing.T, m *model.Model, id string) *model.Shape {
	t.Helper()
	shape, ok := m.Shape(model.MustShapeID(id))
	if !ok {
		t.Fatalf("shape %s not found", id)
	}
	return shape
}

func requireMemberTarget(t *testing.T, m *model.Model, container, member, want string) {
	t.Helper()
	shape := requireShape(t, m, container)
	mbr, ok := shape.GetMember(member)
	if !ok {
		t.Fatalf("%s has no member %q", container, member)
	}
	if got := mbr.Target.String(); got != want {
		t.Errorf("%s$%s target = %s, want %s", container, member, got, want)
	}
}

func requireSyntheticList(t *testing.T, m *model.Model, id string, element string) {
	t.Helper()
	shape := requireShape(t, m, id)
	if shape.Type != model.TypeList {
		t.Fatalf("%s type = %s, want list", id, shape.Type)
	}
	if !shape.HasTrait(model.TraitSynthetic) {
		t.Errorf("%s does not carry @synthetic", id)
	}
	requireMemberTarget(t, m, id, "member", element)
}

func requireSyntheticMap(t *testing.T, m *model.Model, id, key, value string) {
	t.Helper()
	shape := requireShape(t, m, id)
	if shape.Type != model.TypeMap {
		t.Fatalf("%s type = %s, want map", id, shape.Type)
	}
	if !shape.HasTrait(model.TraitSynthetic) {
		t.Errorf("%s does not carry @synthetic", id)
	}
	requireMemberTarget(t, m, id, "key", key)
	requireMemberTarget(t, m, id, "value", value)
}

func TestInlineCollectionsSyntheticShapes(t *testing.T) {
	m := assemble(t, model.Options{},
		filepath.Join(testdataRoot, "loader", "valid", "2.1", "inline-collections.smithy"))
	requireNoDangerFor(t, m)

	requireMemberTarget(t, m, "smithy.example#MyStructure", "strings", "smithy.example#_SyntheticListOf__String")
	requireMemberTarget(t, m, "smithy.example#MyStructure", "tags", "smithy.example#_SyntheticMapOf__String_To___String")
	requireMemberTarget(t, m, "smithy.example#MyStructure", "nested", "smithy.example#_SyntheticMapOf__String_To___SyntheticListOf__Integer")

	requireSyntheticList(t, m, "smithy.example#_SyntheticListOf__String", "smithy.api#String")
	requireSyntheticMap(t, m, "smithy.example#_SyntheticMapOf__String_To___String", "smithy.api#String", "smithy.api#String")
	requireSyntheticList(t, m, "smithy.example#_SyntheticListOf__Integer", "smithy.api#Integer")
	requireSyntheticMap(t, m, "smithy.example#_SyntheticMapOf__String_To___SyntheticListOf__Integer",
		"smithy.api#String", "smithy.example#_SyntheticListOf__Integer")
}

func TestInlineCollectionsLocalTargets(t *testing.T) {
	m := assemble(t, model.Options{},
		filepath.Join(testdataRoot, "loader", "valid", "2.1", "inline-collections-local-target.smithy"))
	requireNoDangerFor(t, m)

	requireMemberTarget(t, m, "smithy.example#MyStructure", "widgets", "smithy.example#_SyntheticListOf_Widget")
	requireMemberTarget(t, m, "smithy.example#MyStructure", "strings", "smithy.example#_SyntheticListOf_String")
	requireSyntheticList(t, m, "smithy.example#_SyntheticListOf_Widget", "smithy.example#Widget")
	// A local forward-declared `String` shadows the prelude.
	requireSyntheticList(t, m, "smithy.example#_SyntheticListOf_String", "smithy.example#String")
}

func TestInlineCollectionsNestedLocalTargets(t *testing.T) {
	m := assemble(t, model.Options{},
		filepath.Join(testdataRoot, "loader", "valid", "2.1", "inline-collections-nested-local-target.smithy"))
	requireNoDangerFor(t, m)

	requireSyntheticMap(t, m, "smithy.example#_SyntheticMapOf_MyKey_To__MyValue",
		"smithy.example#MyKey", "smithy.example#MyValue")
	requireSyntheticMap(t, m, "smithy.example#_SyntheticMapOf__String_To___SyntheticListOf_Widget",
		"smithy.api#String", "smithy.example#_SyntheticListOf_Widget")
	requireSyntheticList(t, m, "smithy.example#_SyntheticListOf_Widget", "smithy.example#Widget")
}

func TestInlineCollectionsUseAndGrouping(t *testing.T) {
	m := assemble(t, model.Options{},
		filepath.Join(testdataRoot, "shared.smithy"),
		filepath.Join(testdataRoot, "loader", "valid", "2.1", "inline-collections-use.smithy"))
	requireNoDangerFor(t, m)
	requireSyntheticList(t, m, "smithy.example#_SyntheticListOf_foo_example_Widget", "foo.example#Widget")

	m = assemble(t, model.Options{},
		filepath.Join(testdataRoot, "loader", "valid", "2.1", "inline-collections-grouping.smithy"))
	requireNoDangerFor(t, m)
	first := requireShape(t, m, "smithy.example#First")
	second := requireShape(t, m, "smithy.example#Second")
	firstName, _ := first.GetMember("names")
	secondTags, _ := second.GetMember("tags")
	if firstName.Target != secondTags.Target {
		t.Errorf("grouped collections target %s and %s, want the same shape", firstName.Target, secondTags.Target)
	}
	if firstName.Target != model.MustShapeID("smithy.example#_SyntheticListOf__String") {
		t.Errorf("grouped collection target = %s", firstName.Target)
	}
}

func TestInlineCollectionsUnionMembers(t *testing.T) {
	m := assemble(t, model.Options{},
		filepath.Join(testdataRoot, "loader", "valid", "2.1", "inline-collections-union.smithy"))
	requireNoDangerFor(t, m)
	requireMemberTarget(t, m, "smithy.example#MyUnion", "names", "smithy.example#_SyntheticListOf__String")
	requireMemberTarget(t, m, "smithy.example#MyUnion", "tags", "smithy.example#_SyntheticMapOf__String_To___Integer")
}

func TestInlineCollectionsGroupAcrossFiles(t *testing.T) {
	m := assembleSources(t,
		`$version: "2.1"
namespace example
structure First {
    names: [String]
}`,
		`$version: "2.1"
namespace example
structure Second {
    tags: [String]
}`)
	requireNoDangerFor(t, m)
	if got := len(m.Shapes()); got == 0 {
		t.Fatal("no shapes")
	}
	first := requireShape(t, m, "example#First")
	second := requireShape(t, m, "example#Second")
	firstName, _ := first.GetMember("names")
	secondTags, _ := second.GetMember("tags")
	if firstName.Target != secondTags.Target || firstName.Target != model.MustShapeID("example#_SyntheticListOf__String") {
		t.Errorf("cross-file collections target %s and %s", firstName.Target, secondTags.Target)
	}
}

func TestInlineCollectionUnresolvedTarget(t *testing.T) {
	m := assembleSources(t, `$version: "2.1"
namespace example

structure S {
    items: [Missing]
}`)
	var unresolved []validation.Event
	for _, e := range validation.Validate(m) {
		if e.Severity == validation.Danger {
			if e.ID != validation.ModelUnresolvedShape {
				t.Errorf("unexpected danger event: %s [%s]", e.String(), e.ID)
			}
			unresolved = append(unresolved, e)
		}
	}
	if len(unresolved) != 1 {
		t.Fatalf("got %d unresolved-target events, want 1: %v", len(unresolved), unresolved)
	}
	if unresolved[0].Source.Line != 5 {
		t.Errorf("unresolved event line = %d, want 5", unresolved[0].Source.Line)
	}
	if !strings.Contains(unresolved[0].Message, "example#Missing") {
		t.Errorf("unresolved event message = %q", unresolved[0].Message)
	}
}

func TestSyntheticShapesRejectTraits(t *testing.T) {
	m := assembleSources(t, `$version: "2.1"
namespace example

structure S {
    items: [String]
}

apply example#_SyntheticListOf__String @sparse
`)
	var found bool
	for _, e := range validation.Validate(m) {
		if e.Severity == validation.Danger && e.ID == validation.ModelInvalidTraitTarget &&
			strings.Contains(e.Message, "synthetic") {
			found = true
		}
	}
	if !found {
		t.Error("applying a trait to a synthetic shape did not produce a danger event")
	}
}

func TestReservedSyntheticNameConflicts(t *testing.T) {
	m := assembleSources(t, `$version: "2.1"
namespace example

structure _SyntheticListOf__String {}

structure S {
    items: [String]
}`)
	var found bool
	for _, e := range validation.Validate(m) {
		if e.Severity == validation.Danger && e.ID == validation.ModelDuplicateShapeId {
			found = true
		}
	}
	if !found {
		t.Error("a user shape with a reserved synthetic name did not produce a duplicate-shape danger")
	}
}

func TestMemberIndexShorthandUsesBuiltinTrait(t *testing.T) {
	m := assembleSources(t, `$version: "2.1"
namespace example

structure S {
    1. first: String
    10. second: Integer
    2147483647. third: Long
}`)
	requireNoDangerFor(t, m)

	definition := requireShape(t, m, "smithy.protocols#idx")
	if !definition.IsTraitDefinition() {
		t.Errorf("smithy.protocols#idx is not a trait definition")
	}
	first := requireShape(t, m, "example#S$first")
	if value, ok := first.GetTrait(model.ProtocolsIdxTrait); !ok {
		t.Error("first has no idx trait")
	} else if n, ok := value.Value.AsInt(); !ok || n != 1 {
		t.Errorf("first idx = %v, %v; want 1", n, ok)
	}
	second := requireShape(t, m, "example#S$second")
	if value, ok := second.GetTrait(model.ProtocolsIdxTrait); !ok {
		t.Error("second has no idx trait")
	} else if n, ok := value.Value.AsInt(); !ok || n != 10 {
		t.Errorf("second idx = %v, %v; want 10", n, ok)
	}
	third := requireShape(t, m, "example#S$third")
	if value, ok := third.GetTrait(model.ProtocolsIdxTrait); !ok {
		t.Error("third has no idx trait")
	} else if n, ok := value.Value.AsInt(); !ok || n != 2147483647 {
		t.Errorf("third idx = %v, %v; want 2147483647", n, ok)
	}
}

func TestMemberIndexShorthandElidedMember(t *testing.T) {
	m := assembleSources(t, `$version: "2.1"
namespace example

@mixin
structure Mixed {
    inherited: String
}

structure S with [Mixed] {
    3. $inherited
}`)
	requireNoDangerFor(t, m)
	member := requireShape(t, m, "example#S$inherited")
	if member.Target != model.MustShapeID("smithy.api#String") {
		t.Errorf("elided member target = %s", member.Target)
	}
	value, ok := member.GetTrait(model.ProtocolsIdxTrait)
	if !ok {
		t.Fatal("elided member has no idx trait")
	}
	if n, ok := value.Value.AsInt(); !ok || n != 3 {
		t.Errorf("elided member idx = %v, %v; want 3", n, ok)
	}
}

func TestMemberIndexUserDefinitionWins(t *testing.T) {
	m := assembleSources(t, `$version: "2.1"
namespace smithy.protocols

@trait(selector: "member")
integer idx

structure S {
    1. first: String
}`)
	requireNoDangerFor(t, m)
	definition := requireShape(t, m, "smithy.protocols#idx")
	if definition.HasTrait(model.MustShapeID("smithy.api#range")) {
		t.Error("user-defined idx was replaced by the built-in definition")
	}
	member := requireShape(t, m, "smithy.protocols#S$first")
	if _, ok := member.GetTrait(model.ProtocolsIdxTrait); !ok {
		t.Error("user-defined idx was not applied to member indexes")
	}
}

func TestMemberIndexesUpstreamFixture(t *testing.T) {
	m := assemble(t, model.Options{},
		filepath.Join(testdataRoot, "loader", "valid", "2.1", "member-indexes.smithy"))
	requireNoDangerFor(t, m)

	indexed := requireShape(t, m, "smithy.protocols#Indexed")
	want := map[string]int64{
		"first":           1,
		"second":          12,
		"index2147483644": 2147483644,
		"index2147483645": 2147483645,
		"index2147483646": 2147483646,
		"equal":           20,
		"inherited":       3,
		"values":          2147483647,
	}
	for name, wantIdx := range want {
		member, ok := indexed.GetMember(name)
		if !ok {
			t.Errorf("Indexed has no member %q", name)
			continue
		}
		trait, ok := member.GetTrait(model.ProtocolsIdxTrait)
		if !ok {
			t.Errorf("Indexed$%s has no idx trait", name)
			continue
		}
		if got, ok := trait.Value.AsInt(); !ok || got != wantIdx {
			t.Errorf("Indexed$%s idx = %v, %v; want %d", name, got, ok, wantIdx)
		}
	}
	values, _ := indexed.GetMember("values")
	if got := values.Target.String(); got != "smithy.protocols#_SyntheticListOf__String" {
		t.Errorf("Indexed$values target = %s", got)
	}
	requireSyntheticList(t, m, "smithy.protocols#_SyntheticListOf__String", "smithy.api#String")
}
