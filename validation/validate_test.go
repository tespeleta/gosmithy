package validation_test

import (
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

func assemble(t *testing.T, src string) *model.Model {
	t.Helper()
	f, err := idl.ParseWithOptions("test.smithy", src, idl.Options{
		Version:        idl.V2_1,
		DefaultVersion: idl.V2_0,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m, err := model.Assemble([]*idl.File{f}, model.Options{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return m
}

func findEvent(t *testing.T, events []validation.Event, id string) validation.Event {
	t.Helper()
	for _, e := range events {
		if e.ID == id {
			return e
		}
	}
	var got []string
	for _, e := range events {
		got = append(got, e.ID)
	}
	t.Fatalf("event %s not found; got %v", id, got)
	return validation.Event{}
}

func TestUnresolvedTargetLocation(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
structure A {
    b: Missing
}
`)
	events := validation.Validate(m)
	e := findEvent(t, events, validation.ModelUnresolvedShape)
	if e.Severity != validation.Danger {
		t.Errorf("severity = %s", e.Severity)
	}
	if e.Source.Filename != "test.smithy" || e.Source.Line != 4 {
		t.Errorf("source = %s, want test.smithy:4", e.Source)
	}
	if e.ShapeID != model.MustShapeID("example#A$b") {
		t.Errorf("shape ID = %s", e.ShapeID)
	}
	if !strings.Contains(e.Message, "example#Missing") {
		t.Errorf("message = %q", e.Message)
	}
	if !strings.HasPrefix(e.String(), "test.smithy:4:5: DANGER: ") {
		t.Errorf("rendered event = %q", e.String())
	}
}

func TestInvalidMemberTarget(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
operation Op {}
structure A {
    op: Op
}
`)
	e := findEvent(t, validation.Validate(m), validation.ModelInvalidMemberTarget)
	if !strings.Contains(e.Message, "operation") {
		t.Errorf("message = %q", e.Message)
	}
}

func TestInvalidOperationIO(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
operation Op {
    input: String
    output: NotAStructure
}
string NotAStructure
`)
	events := validation.Validate(m)
	e := findEvent(t, events, validation.ModelInvalidOperationIo)
	if !strings.Contains(e.Message, "structure or smithy.api#Unit") {
		t.Errorf("message = %q", e.Message)
	}
}

func TestInvalidOperationError(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
operation Op {
    errors: [NotAnError]
}
structure NotAnError {}
`)
	e := findEvent(t, validation.Validate(m), validation.ModelInvalidOperationErr)
	if !strings.Contains(e.Message, "@error") {
		t.Errorf("message = %q", e.Message)
	}
}

func TestDuplicateMemberNamesAreCaseInsensitive(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
structure A {
    foo: String
    FOO: Integer
}
`)
	e := findEvent(t, validation.Validate(m), validation.ModelInvalidMember)
	if e.Source.Line != 5 {
		t.Errorf("event line = %d, want 5", e.Source.Line)
	}
}

func TestInvalidTraitTarget(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
@required
string A
`)
	e := findEvent(t, validation.Validate(m), validation.ModelInvalidTraitTarget)
	if !strings.Contains(e.Message, "required") || !strings.Contains(e.Message, "members") {
		t.Errorf("message = %q", e.Message)
	}
}

func TestSyntacticShapeID(t *testing.T) {
	m := assemble(t, `$version: "2.0"
metadata value = missing
namespace example
`)
	e := findEvent(t, validation.Validate(m), validation.SyntacticShapeIdTarget)
	if !strings.Contains(e.Message, "`missing`") || !strings.Contains(e.Message, "`smithy.api#missing`") {
		t.Errorf("message = %q", e.Message)
	}
	if e.Source.Line != 2 {
		t.Errorf("event line = %d", e.Source.Line)
	}
}

func TestServiceAndResourceReferences(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
service Service {
    version: "1"
    operations: [NotAnOperation]
}
structure NotAnOperation {}

resource Resource {
    read: NotAnOperation
}
`)
	events := validation.Validate(m)
	findEvent(t, events, validation.ModelInvalidReference)
	findEvent(t, events, validation.ModelInvalidResource)
}

func TestMixinMustBeMarked(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
structure A {}
structure B with [A] {}
`)
	e := findEvent(t, validation.Validate(m), validation.ModelInvalidMixin)
	if !strings.Contains(e.Message, "not a mixin") {
		t.Errorf("message = %q", e.Message)
	}
}

func TestMixinCycle(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
@mixin
structure A with [B] {}
@mixin
structure B with [A] {}
`)
	e := findEvent(t, validation.Validate(m), validation.ModelInvalidMixin)
	if !strings.Contains(e.Message, "cycle") {
		t.Errorf("message = %q", e.Message)
	}
}

func TestValidModelHasNoEvents(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
service Service {
    version: "1"
    operations: [Op]
}
operation Op {
    input := { value: String }
    output := { value: String }
}
`)
	if events := validation.Validate(m); len(events) != 0 {
		for _, e := range events {
			t.Errorf("unexpected event: %s [%s]", e.String(), e.ID)
		}
	}
}

func TestEventsAreDeterministicallySorted(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example
structure A {
    z: MissingZ
    a: MissingA
}
structure B {
    b: MissingB
}
`)
	first := validation.Validate(m)
	for i := 0; i < 5; i++ {
		next := validation.Validate(m)
		if len(next) != len(first) {
			t.Fatalf("event count changed")
		}
		for j := range first {
			if first[j].String() != next[j].String() {
				t.Fatalf("event order changed at %d", j)
			}
		}
	}
	for i := 1; i < len(first); i++ {
		prev, cur := first[i-1], first[i]
		if prev.Source.Line > cur.Source.Line {
			t.Errorf("events are not sorted by line: %d then %d", prev.Source.Line, cur.Source.Line)
		}
	}
}

func TestParseSeverity(t *testing.T) {
	for input, want := range map[string]validation.Severity{
		"note":    validation.Note,
		"WARNING": validation.Warning,
		"danger":  validation.Danger,
		"error":   validation.Danger,
	} {
		got, ok := validation.ParseSeverity(input)
		if !ok || got != want {
			t.Errorf("ParseSeverity(%q) = %v, %v", input, got, ok)
		}
	}
	if _, ok := validation.ParseSeverity("bogus"); ok {
		t.Errorf("ParseSeverity accepted an invalid value")
	}
}

func TestBigDecimalRangeBoundsAccepted(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example

@range(min: 0.000000000000000000000000000001, max: 100.500000000000000000000000000001)
bigDecimal Precise

structure In {
    value: Precise
}
`)
	for _, e := range validation.Validate(m) {
		if e.Severity == validation.Danger {
			t.Errorf("unexpected danger: %s", e.String())
		}
	}
}

func TestTraitRangeComparisonIsExact(t *testing.T) {
	m := assemble(t, `$version: "2.0"
namespace example

@trait(selector: "string")
structure limit {
    @range(min: 10000000000000000000000000000000000000001)
    value: BigInteger
}

@limit(value: 10000000000000000000000000000000000000000)
string S
`)
	events := validation.Validate(m)
	found := false
	for _, e := range events {
		if e.ID == validation.ModelInvalidTraitValue && strings.Contains(e.Message, "less than the minimum") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want exact range violation, got %v", events)
	}
}
