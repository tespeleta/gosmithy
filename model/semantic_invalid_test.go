package model_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

// semanticInvalidCase describes the expected semantic failure of a fixture in
// testdata/loader/invalid. The expected message comes from the fixture's first
// line unless message is set, and line is the source line the event must point
// at.
type semanticInvalidCase struct {
	path    string
	id      string
	message string
	line    int
}

var semanticInvalidFixtures = []semanticInvalidCase{
	{"2.1/inline-collection-unresolved-target-list.smithy", validation.ModelUnresolvedShape, "smithy.example#Missing", 7},
	{"2.1/inline-collection-unresolved-target-map-key.smithy", validation.ModelUnresolvedShape, "smithy.example#Missing", 7},
	{"2.1/inline-collection-unresolved-target-map-value.smithy", validation.ModelUnresolvedShape, "smithy.example#Missing", 7},
	{"elision/elide-nonexistent-id.smithy", validation.ModelInvalidMember, "", 9},
	{"elision/elided-property-type-conflicts.smithy", validation.ModelInvalidMixin, "", 29},
	{"elision/elided-type-conflicts.smithy", validation.ModelInvalidMixin, "", 18},
	{"elision/for-referencing-non-resource.smithy", validation.ModelInvalidReference, "", 11},
	{"enums/enum-with-bad-values.smithy", validation.ModelInvalidEnum, "", 7},
	{"enums/enum-without-member.smithy", validation.ModelInvalidEnum, "", 7},
	{"enums/idl-enums-in-v1.smithy", validation.ModelInvalidEnum, "", 6},
	{"enums/int-enum-without-member.smithy", validation.ModelInvalidEnum, "", 7},
	{"enums/intEnum-with-bad-value.smithy", validation.ModelInvalidEnum, "", 7},
	{"enums/intEnum-with-float.smithy", validation.ModelInvalidEnum, "", 7},
	{"enums/intEnum-with-long.smithy", validation.ModelInvalidEnum, "", 8},
	{"lists/list-empty-members.smithy", validation.ModelInvalidMember, "", 4},
	{"lists/no-sets-in-v2.smithy", validation.ModelInvalidShapeType, "", 6},
	{"lists/set-empty-members.smithy", validation.ModelInvalidMember, "", 4},
	{"maps/map-empty-members.smithy", validation.ModelInvalidMember, "", 4},
	{"metadata/space-after-node-array-keyword.smithy", validation.SyntacticShapeIdTarget, "", 3},
	{"mixins/member-mixin-conflict-1.smithy", validation.ModelInvalidMixin, "", 17},
	{"mixins/member-mixin-conflict-2.smithy", validation.ModelInvalidMixin, "", 8},
	{"mixins/mix-types.smithy", validation.ModelInvalidMixin, "", 10},
	{"mixins/mixins-trait-in-explicit-1-0.smithy", validation.ModelInvalidMixin, "", 5},
	{"mixins/mixins-trait-in-implicit-1-0.smithy", validation.ModelInvalidMixin, "", 4},
	{"mixins/operation-mixin-with-io.smithy", validation.ModelInvalidMixin, "", 7},
	{"mixins/resource-mixin-with-properties.smithy", validation.ModelInvalidMixin, "", 8},
	{"version/idl-int-enums-in-v1.smithy", validation.ModelInvalidEnum, "", 6},
}

func TestSemanticInvalidFixtures(t *testing.T) {
	for _, tc := range semanticInvalidFixtures {
		t.Run(tc.path, func(t *testing.T) {
			path := filepath.Join(testdataRoot, "loader", "invalid", tc.path)
			expected := tc.message
			if expected == "" {
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				firstLine, _, _ := strings.Cut(string(src), "\n")
				expected = strings.TrimSpace(strings.TrimPrefix(firstLine, "//"))
				if before, _, ok := strings.Cut(expected, " | "); ok {
					expected = before
				}
			}

			file := parseFile(t, path)
			m, err := model.Assemble([]*idl.File{file}, model.Options{})
			if err != nil {
				t.Fatal(err)
			}
			events := validation.Validate(m)

			var dangers []validation.Event
			for _, e := range events {
				if e.Severity == validation.Danger {
					dangers = append(dangers, e)
				}
			}
			if len(dangers) != 1 {
				for _, e := range dangers {
					t.Logf("event: [%s] %s", e.ID, e.FullMessage())
				}
				t.Fatalf("got %d danger events, want 1", len(dangers))
			}
			event := dangers[0]
			if event.ID != tc.id {
				t.Errorf("event ID = %q, want %q (message: %s)", event.ID, tc.id, event.FullMessage())
			}
			if !strings.Contains(event.FullMessage(), expected) {
				t.Errorf("event message %q does not contain %q", event.FullMessage(), expected)
			}
			if event.Source.Filename != filepath.Base(path) {
				t.Errorf("event filename = %q, want %q", event.Source.Filename, filepath.Base(path))
			}
			if event.Source.Line != tc.line {
				t.Errorf("event line = %d, want %d", event.Source.Line, tc.line)
			}
		})
	}
}
