package validation_test

import (
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/validation"
)

// validationDangerCases are invalid models that must be rejected by
// validation.Validate. Substrings follow the stable-fragment convention: the
// severity must be DANGER and the message must name the problem.
var validationDangerCases = []struct {
	name   string
	source string
	substr string
}{
	{
		name: "map_key_not_string",
		source: `$version: "2.0"
namespace tdd

map M {
    key: Integer
    value: String
}

structure In {
    m: M
}`,
		substr: "map key",
	},
	{
		name: "resource_identifier_not_string",
		source: `$version: "2.0"
namespace tdd

resource R {
    identifiers: {
        id: Integer
    }
}

service S {
    version: "1.0.0"
    resources: [R]
}`,
		substr: "identifier",
	},
	{
		name: "fractional_range_on_integer",
		source: `$version: "2.0"
namespace tdd

@range(min: 1.5)
integer Bad

structure In {
    bad: Bad
}`,
		substr: "range",
	},
	{
		name: "duplicate_json_name",
		source: `$version: "2.0"
namespace tdd

structure In {
    @jsonName("x")
    a: String

    @jsonName("x")
    b: String
}`,
		substr: "jsonName",
	},
	{
		name: "http_code_out_of_range",
		source: `$version: "2.0"
namespace tdd

@http(method: "GET", uri: "/x", code: 42)
operation Op {}

service S {
    version: "1.0.0"
    operations: [Op]
}`,
		substr: "42",
	},
}

func TestValidationDangers(t *testing.T) {
	for _, tc := range validationDangerCases {
		t.Run(tc.name, func(t *testing.T) {
			m := assemble(t, tc.source)
			requireDangerContaining(t, validation.Validate(m), tc.substr)
		})
	}
}

func requireDangerContaining(t *testing.T, events []validation.Event, substr string) {
	t.Helper()
	for _, e := range events {
		if e.Severity == validation.Danger &&
			strings.Contains(strings.ToLower(e.FullMessage()), strings.ToLower(substr)) {
			return
		}
	}
	t.Fatalf("want a DANGER containing %q, got %v", substr, events)
}
