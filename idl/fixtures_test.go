package idl_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
)

// maxVersion is the newest IDL version exercised by the fixture harness.
// M1 implements the full 2.0 grammar plus the 2.1 syntax additions
// (member indexes, inline collections, and tagged literals).
var maxVersion = idl.V2_1

// skipValid lists valid fixtures that are intentionally not exercised yet.
var skipValid = map[string]bool{}

// skipInvalid lists invalid fixtures whose expected failure is produced by
// semantic assembly or validation rather than the parser. They are M2 work.
var skipInvalid = map[string]bool{
	// Semantic: target elision, mixin application, and resource binding.
	"invalid/elision/elide-nonexistent-id.smithy":           true,
	"invalid/elision/elided-property-type-conflicts.smithy": true,
	"invalid/elision/elided-type-conflicts.smithy":          true,
	"invalid/elision/for-referencing-non-resource.smithy":   true,

	// Semantic: enum value validation.
	"invalid/enums/enum-with-bad-values.smithy":    true,
	"invalid/enums/enum-without-member.smithy":     true,
	"invalid/enums/idl-enums-in-v1.smithy":         true,
	"invalid/enums/int-enum-without-member.smithy": true,
	"invalid/enums/intEnum-with-bad-value.smithy":  true,
	"invalid/enums/intEnum-with-float.smithy":      true,
	"invalid/enums/intEnum-with-long.smithy":       true,

	// Semantic: required members and version-specific shape types.
	"invalid/lists/list-empty-members.smithy":    true,
	"invalid/lists/no-sets-in-v2.smithy":         true,
	"invalid/lists/set-empty-members.smithy":     true,
	"invalid/maps/map-empty-members.smithy":      true,
	"invalid/version/idl-int-enums-in-v1.smithy": true,

	// Semantic: syntactic shape ID resolution.
	"invalid/metadata/space-after-node-array-keyword.smithy": true,

	// Semantic: mixin conflict and compatibility validation.
	"invalid/mixins/member-mixin-conflict-1.smithy":        true,
	"invalid/mixins/member-mixin-conflict-2.smithy":        true,
	"invalid/mixins/mix-types.smithy":                      true,
	"invalid/mixins/mixins-trait-in-explicit-1-0.smithy":   true,
	"invalid/mixins/mixins-trait-in-implicit-1-0.smithy":   true,
	"invalid/mixins/operation-mixin-with-io.smithy":        true,
	"invalid/mixins/resource-mixin-with-properties.smithy": true,

	// Semantic: inline collection targets are unresolved; resolution is M2.
	"invalid/2.1/inline-collection-unresolved-target-list.smithy":      true,
	"invalid/2.1/inline-collection-unresolved-target-map-key.smithy":   true,
	"invalid/2.1/inline-collection-unresolved-target-map-value.smithy": true,
}

const fixtureRoot = "../testdata/loader"

func TestValidFixtures(t *testing.T) {
	root := filepath.Join(fixtureRoot, "valid")
	var count int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".smithy") {
			return nil
		}
		rel, err := filepath.Rel(fixtureRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if skipValid[rel] {
			return nil
		}
		count++
		t.Run(rel, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := idl.ParseWithOptions(rel, string(src), idl.Options{
				Version:        maxVersion,
				DefaultVersion: idl.V1_0,
			}); err != nil {
				t.Fatalf("expected fixture to parse: %v", err)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 100 {
		t.Fatalf("parsed %d valid fixtures, want 100", count)
	}
	t.Logf("parsed %d valid fixtures (skipped %d)", count, len(skipValid))
}

func TestInvalidFixtures(t *testing.T) {
	root := filepath.Join(fixtureRoot, "invalid")
	var total, skipped, checked int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".smithy") {
			return nil
		}
		rel, err := filepath.Rel(fixtureRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		total++

		if skipInvalid[rel] {
			skipped++
			return nil
		}
		checked++

		t.Run(rel, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			firstLine, _, _ := strings.Cut(string(src), "\n")
			if !strings.HasPrefix(firstLine, "//") {
				t.Fatalf("fixture does not start with an expected-error comment: %q", firstLine)
			}
			expected := strings.TrimSpace(strings.TrimPrefix(firstLine, "//"))
			expected = normalizeMessage(expected)

			_, parseErr := idl.ParseWithOptions(rel, string(src), idl.Options{
				Version:        maxVersion,
				DefaultVersion: idl.V1_0,
			})
			if parseErr == nil {
				t.Fatalf("expected parse error containing %q, but parsing succeeded", expected)
			}
			actual := normalizeMessage(parseErr.Error())
			if !strings.Contains(actual, expected) {
				t.Fatalf("expected parse error containing:\n  %s\nactual:\n  %s", expected, actual)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 207 || checked != 180 || skipped != 27 {
		t.Fatalf("checked=%d skipped=%d total=%d, want 180/27/207", checked, skipped, total)
	}
	t.Logf("checked %d invalid fixtures (skipped %d semantic, total %d)", checked, skipped, total)
}

func normalizeMessage(s string) string {
	s = strings.ReplaceAll(s, "[EOF]", "")
	s = strings.ReplaceAll(s, " | Model", "")
	s = strings.ReplaceAll(s, "\r\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
