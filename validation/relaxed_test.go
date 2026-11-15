package validation_test

import (
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
	"github.com/tespeleta/gosmithy/validation"
)

func assembleRelaxed(t *testing.T, src string) *model.Model {
	t.Helper()
	file, err := idl.ParseWithOptions("relaxed.smithy", src, idl.Options{
		Version:        idl.V2_1,
		DefaultVersion: idl.V2_0,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	definition, err := httpjson.Definition()
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.Assemble([]*idl.File{definition, file}, model.Options{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return m
}

func TestRelaxedInputValidTargets(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "service",
			source: `$version: "2.0"
namespace tdd
use gosmithy.protocols#relaxedInput

@relaxedInput
service S { operations: [Op] }

operation Op { input: In }

structure In {
    ok: Boolean
}`,
		},
		{
			name: "member",
			source: `$version: "2.0"
namespace tdd
use gosmithy.protocols#relaxedInput

service S { operations: [Op] }

operation Op { input: In }

structure In {
    @relaxedInput
    count: Integer
}`,
		},
		{
			name: "member_opt_out",
			source: `$version: "2.0"
namespace tdd
use gosmithy.protocols#relaxedInput

service S { operations: [Op] }

operation Op { input: In }

structure In {
    @relaxedInput(enabled: false)
    ok: Boolean
}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := assembleRelaxed(t, tc.source)
			for _, event := range validation.Validate(m) {
				if event.Severity == validation.Danger {
					t.Fatalf("unexpected DANGER: %s", event.String())
				}
			}
		})
	}
}

func TestRelaxedInputInvalidTargets(t *testing.T) {
	cases := []struct {
		name   string
		source string
		substr string
	}{
		{
			name: "non_scalar_member",
			source: `$version: "2.0"
namespace tdd
use gosmithy.protocols#relaxedInput

structure In {
    @relaxedInput
    name: String
}`,
			substr: "boolean, numeric, or bigDecimal",
		},
		{
			name: "collection_member",
			source: `$version: "2.0"
namespace tdd
use gosmithy.protocols#relaxedInput

list L {
    @relaxedInput
    member: Integer
}`,
			substr: "structure or union members",
		},
		{
			name: "operation",
			source: `$version: "2.0"
namespace tdd
use gosmithy.protocols#relaxedInput

@relaxedInput
operation Op {}`,
			substr: "service or a member",
		},
		{
			name: "wrong_value_type",
			source: `$version: "2.0"
namespace tdd
use gosmithy.protocols#relaxedInput

@relaxedInput(enabled: "yes")
service S { operations: [Op] }

operation Op {}`,
			substr: "expected a boolean",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := assembleRelaxed(t, tc.source)
			found := false
			for _, event := range validation.Validate(m) {
				if event.Severity == validation.Danger && strings.Contains(strings.ToLower(event.FullMessage()), strings.ToLower(tc.substr)) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("want a DANGER containing %q, got %v", tc.substr, validation.Validate(m))
			}
		})
	}
}
