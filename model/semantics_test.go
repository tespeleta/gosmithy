package model_test

import (
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

func assembleSources(t *testing.T, sources ...string) *model.Model {
	t.Helper()
	files := make([]*idl.File, 0, len(sources))
	for i, src := range sources {
		name := "test.smithy"
		if len(sources) > 1 {
			name = "test" + string(rune('a'+i)) + ".smithy"
		}
		f, err := idl.ParseWithOptions(name, src, idl.Options{
			Version:        idl.V2_1,
			DefaultVersion: idl.V2_0,
		})
		if err != nil {
			t.Fatalf("parsing source: %v", err)
		}
		files = append(files, f)
	}
	m, err := model.Assemble(files, model.Options{})
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	return m
}

func requireNoDanger(t *testing.T, m *model.Model) {
	t.Helper()
	for _, e := range validation.Validate(m) {
		if e.Severity == validation.Danger {
			t.Errorf("unexpected danger event: %s [%s]", e.String(), e.ID)
		}
	}
}

func eventIDs(m *model.Model) []string {
	var ids []string
	for _, e := range validation.Validate(m) {
		ids = append(ids, e.ID)
	}
	return ids
}

func hasEvent(m *model.Model, id string) bool {
	for _, got := range eventIDs(m) {
		if got == id {
			return true
		}
	}
	return false
}

func TestForwardReferencesAndUse(t *testing.T) {
	m := assembleSources(t,
		`$version: "2.0"
namespace example
use other.ns#Widget
structure A { b: B }
structure B { w: Widget }
string String
structure C { s: String }
`,
		`$version: "2.0"
namespace other.ns
structure Widget { id: String }
`)
	requireNoDanger(t, m)
	a, _ := m.Shape(model.MustShapeID("example#A"))
	aMember, _ := a.GetMember("b")
	if aMember.Target != model.MustShapeID("example#B") {
		t.Errorf("A$b target = %s", aMember.Target)
	}
	b, _ := m.Shape(model.MustShapeID("example#B"))
	bMember, _ := b.GetMember("w")
	if bMember.Target != model.MustShapeID("other.ns#Widget") {
		t.Errorf("B$w target = %s", bMember.Target)
	}
	// The current namespace wins over the prelude.
	c, _ := m.Shape(model.MustShapeID("example#C"))
	cMember, _ := c.GetMember("s")
	if cMember.Target != model.MustShapeID("example#String") {
		t.Errorf("C$s target = %s, want example#String", cMember.Target)
	}
}

func TestUseStatementUnresolved(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
use missing.ns#Gone
`)
	if !hasEvent(m, validation.ModelUnresolvedShape) {
		t.Fatalf("expected Model.UnresolvedShape, got %v", eventIDs(m))
	}
	for _, e := range validation.Validate(m) {
		if e.ID == validation.ModelUnresolvedShape {
			if e.Source.Line != 3 {
				t.Errorf("use event line = %d, want 3", e.Source.Line)
			}
			if !strings.Contains(e.Message, "missing.ns#Gone") {
				t.Errorf("use event message = %q", e.Message)
			}
		}
	}
}

func TestInlineOperationIO(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
operation Op {
    input := { a: String }
    output := { b: Integer }
}
`)
	requireNoDanger(t, m)
	op, _ := m.Shape(model.MustShapeID("example#Op"))
	if op.Input != model.MustShapeID("example#OpInput") {
		t.Errorf("Op input = %s", op.Input)
	}
	if op.Output != model.MustShapeID("example#OpOutput") {
		t.Errorf("Op output = %s", op.Output)
	}
	input, ok := m.Shape(model.MustShapeID("example#OpInput"))
	if !ok || input.Type != model.TypeStructure {
		t.Fatalf("OpInput missing or wrong type")
	}
	if !input.IsInput() {
		t.Errorf("OpInput should have @input")
	}
	if input.Source.Line != 3 {
		t.Errorf("OpInput source line = %d, want 3 (the operation)", input.Source.Line)
	}
	member, _ := input.GetMember("a")
	if member.Target != model.MustShapeID("smithy.api#String") {
		t.Errorf("OpInput$a target = %s", member.Target)
	}
	output, _ := m.Shape(model.MustShapeID("example#OpOutput"))
	if !output.IsOutput() {
		t.Errorf("OpOutput should have @output")
	}
}

func TestInlineOperationIOSuffix(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
$operationInputSuffix: "Request"
$operationOutputSuffix: "Response"
namespace example
operation Op {
    input := {}
    output := {}
}
`)
	requireNoDanger(t, m)
	op, _ := m.Shape(model.MustShapeID("example#Op"))
	if op.Input != model.MustShapeID("example#OpRequest") {
		t.Errorf("Op input = %s", op.Input)
	}
	if op.Output != model.MustShapeID("example#OpResponse") {
		t.Errorf("Op output = %s", op.Output)
	}
}

func TestOperationDefaultUnit(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
structure In {}
operation Explicit { input: In }
operation Implicit {}
`)
	requireNoDanger(t, m)
	explicit, _ := m.Shape(model.MustShapeID("example#Explicit"))
	if explicit.Input != model.MustShapeID("example#In") {
		t.Errorf("Explicit input = %s", explicit.Input)
	}
	if explicit.Output != model.TraitUnit {
		t.Errorf("Explicit output = %s", explicit.Output)
	}
	implicit, _ := m.Shape(model.MustShapeID("example#Implicit"))
	if implicit.Input != model.TraitUnit || implicit.Output != model.TraitUnit {
		t.Errorf("Implicit IO = %s, %s", implicit.Input, implicit.Output)
	}
}

func TestApplyStatements(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
structure Foo {
    bar: String
}
apply Foo$bar @documentation("member doc")
apply Foo {
    @deprecated
    @internal
}
`)
	requireNoDanger(t, m)
	foo, _ := m.Shape(model.MustShapeID("example#Foo"))
	if !foo.IsDeprecated() || !foo.HasTrait(model.TraitInternal) {
		t.Errorf("block apply traits missing: %v", foo.Traits.SortedIDs())
	}
	bar, _ := foo.GetMember("bar")
	if doc, ok := bar.Documentation(); !ok || doc != "member doc" {
		t.Errorf("member documentation = %q, %v", doc, ok)
	}
}

func TestTraitAnnotationCoercion(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
@tags
string A
@references
string B
@deprecated
string C
@documentation
string D
`)
	requireNoDangerExcept(t, m, validation.ModelInvalidTraitValue)
	a, _ := m.Shape(model.MustShapeID("example#A"))
	tags, _ := a.GetTrait(model.TraitTags)
	if tags.Value.Kind != model.NodeArray || len(tags.Value.Arr) != 0 {
		t.Errorf("@tags annotation coerced to %s", tags.Value)
	}
	b, _ := m.Shape(model.MustShapeID("example#B"))
	refs, _ := b.GetTrait(model.TraitReferences)
	if refs.Value.Kind != model.NodeArray || len(refs.Value.Arr) != 0 {
		t.Errorf("@references annotation coerced to %s", refs.Value)
	}
	c, _ := m.Shape(model.MustShapeID("example#C"))
	dep, _ := c.GetTrait(model.TraitDeprecated)
	if dep.Value.Kind != model.NodeObject {
		t.Errorf("@deprecated annotation coerced to %s", dep.Value)
	}
	if !hasEvent(m, validation.ModelInvalidTraitValue) {
		t.Errorf("scalar annotation should produce Model.InvalidTraitValue, got %v", eventIDs(m))
	}
}

func requireNoDangerExcept(t *testing.T, m *model.Model, allowed ...string) {
	t.Helper()
	allow := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		allow[id] = true
	}
	for _, e := range validation.Validate(m) {
		if e.Severity == validation.Danger && !allow[e.ID] {
			t.Errorf("unexpected danger event: %s [%s]", e.String(), e.ID)
		}
	}
}

func TestMetadataMerging(t *testing.T) {
	m := assembleSources(t,
		`$version: "2.0"
metadata list = ["a"]
metadata same = "x"
metadata obj = {a: 1}
`,
		`$version: "2.0"
metadata list = ["b"]
metadata same = "x"
`)
	requireNoDanger(t, m)
	list, _ := m.Metadata("list")
	if list.Kind != model.NodeArray || len(list.Arr) != 2 || list.Arr[0].Str != "a" || list.Arr[1].Str != "b" {
		t.Errorf("merged list = %s", list)
	}

	conflict := assembleSources(t,
		`$version: "2.0"
metadata value = "a"
`,
		`$version: "2.0"
metadata value = "b"
`)
	if !hasEvent(conflict, validation.ModelMetadataConflict) {
		t.Errorf("expected metadata conflict, got %v", eventIDs(conflict))
	}

	syntactic := assembleSources(t, `$version: "2.0"
metadata example = required
`)
	requireNoDanger(t, syntactic)
	value, _ := syntactic.Metadata("example")
	if value.Str != "smithy.api#required" || !value.SyntacticShapeID || value.Raw != "required" {
		t.Errorf("syntactic metadata value = %#v", value)
	}
}

func TestEnumSugar(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
enum Color {
    RED
    GREEN = "green"
}
intEnum Code {
    OK = 200
    ERROR = 500
}
`)
	requireNoDanger(t, m)
	color, _ := m.Shape(model.MustShapeID("example#Color"))
	red, _ := color.GetMember("RED")
	if value, _ := red.EnumValue(); value.Str != "RED" {
		t.Errorf("RED enum value = %s", value)
	}
	green, _ := color.GetMember("GREEN")
	if value, _ := green.EnumValue(); value.Str != "green" {
		t.Errorf("GREEN enum value = %s", value)
	}
	code, _ := m.Shape(model.MustShapeID("example#Code"))
	ok200, _ := code.GetMember("OK")
	if value, _ := ok200.EnumValue(); value.Num != "200" {
		t.Errorf("OK enum value = %s", value)
	}
}

func TestDuplicateShapeMerge(t *testing.T) {
	merged := assembleSources(t,
		`$version: "2.0"
namespace example
structure Foo {
    a: String
}
`,
		`$version: "2.0"
namespace example
@deprecated
structure Foo {
    a: String
}
`)
	requireNoDanger(t, merged)
	foo, _ := merged.Shape(model.MustShapeID("example#Foo"))
	if !foo.IsDeprecated() {
		t.Errorf("duplicate shape traits were not merged")
	}
	if len(foo.Members) != 1 {
		t.Errorf("duplicate shape members = %d", len(foo.Members))
	}

	conflict := assembleSources(t,
		`$version: "2.0"
namespace example
structure Foo { a: String }
`,
		`$version: "2.0"
namespace example
structure Foo { a: Integer }
`)
	if !hasEvent(conflict, validation.ModelInvalidMember) {
		t.Errorf("expected member conflict, got %v", eventIDs(conflict))
	}

	typeConflict := assembleSources(t,
		`$version: "2.0"
namespace example
structure Foo {}
`,
		`$version: "2.0"
namespace example
string Foo
`)
	if !hasEvent(typeConflict, validation.ModelDuplicateShapeId) {
		t.Errorf("expected duplicate shape ID event, got %v", eventIDs(typeConflict))
	}

	caseConflict := assembleSources(t,
		`$version: "2.0"
namespace example
structure Foo {}
`,
		`$version: "2.0"
namespace example
structure foo {}
`)
	if !hasEvent(caseConflict, validation.ModelDuplicateShapeId) {
		t.Errorf("expected case-insensitive duplicate event, got %v", eventIDs(caseConflict))
	}
}

func TestServiceClosureSkipsTraitReferences(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
service Service {
    version: "1"
    operations: [Op]
    errors: [CommonError]
}
@documentation(Hidden)
operation Op {
    errors: [OpError]
}
@error("client")
structure CommonError {}
@error("server")
structure OpError {}
structure Hidden {}
`)
	requireNoDanger(t, m)
	closure, ok := m.ServiceClosure(model.MustShapeID("example#Service"))
	if !ok {
		t.Fatal("closure missing")
	}
	inClosure := make(map[model.ShapeID]bool, len(closure))
	for _, id := range closure {
		inClosure[id] = true
	}
	if !inClosure[model.MustShapeID("example#OpError")] {
		t.Errorf("operation error should be in closure")
	}
	if !inClosure[model.MustShapeID("example#CommonError")] {
		t.Errorf("service error should be in closure")
	}
	if inClosure[model.MustShapeID("example#Hidden")] {
		t.Errorf("trait-only reference must not be in the closure")
	}
	effective := m.ServiceOperationErrors(model.MustShapeID("example#Service"), model.MustShapeID("example#Op"))
	if len(effective) != 2 {
		t.Errorf("effective errors = %v, want both common and operation errors", effective)
	}
}

func TestAllMemberShapesHaveSourceLocations(t *testing.T) {
	m := assemble(t, model.Options{}, "../testdata/weather.smithy")
	for _, s := range m.Shapes() {
		if !s.IsMember() {
			continue
		}
		if s.Source.Line == 0 || s.Source.Filename == "" {
			t.Errorf("member %s has no source location", s.ID)
		}
		container, ok := m.Shape(s.Container)
		if !ok {
			t.Errorf("member %s has unknown container %s", s.ID, s.Container)
			continue
		}
		if _, ok := container.GetMember(s.ID.Member); !ok {
			t.Errorf("member %s is not reachable from its container", s.ID)
		}
	}
}
