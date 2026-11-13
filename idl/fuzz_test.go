package idl

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzParse checks that arbitrary input never panics or hangs. Parse errors
// are expected and ignored.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		"$version: \"2.0\"\nnamespace a.b\nstring S\n",
		"metadata foo = {a: [1, true, null, \"x\"], b: c}\nnamespace a\n",
		"$version: \"2.1\"\nnamespace a\nstructure S { 1. a: [String] }\n",
		"$version: \"2.1\"\nnamespace a\n@pattern(#re \"^x$\") string S\n",
		"/// docs\n$version: \"2.0\"\nnamespace a\n@trait(selector: \"*\") structure t {}\n",
		"@documentation(\"\"\"\n  hi\n  \"\"\")\nnamespace a\nstring S\n",
		"apply SomeShape {\n @a @b\n}\nnamespace a\n",
		"namespace a\noperation O { input := {a: String} output := {b: String} errors: [E] }\n",
		"namespace a\nresource R { identifiers: {id: String} read: O }\n",
		"namespace a\nunion U { a: String b: Integer }\n",
	}

	// Seed the corpus from a sample of the real fixtures.
	for _, dir := range []string{"valid", "invalid"} {
		root := filepath.Join("..", "testdata", "loader", dir)
		count := 0
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || count >= 50 {
				return nil
			}
			if filepath.Ext(path) != ".smithy" {
				return nil
			}
			if b, readErr := os.ReadFile(path); readErr == nil {
				seeds = append(seeds, string(b))
				count++
			}
			return nil
		})
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		_, _ = ParseWithOptions("fuzz.smithy", src, Options{
			Version:        V2_1,
			DefaultVersion: V2_0,
		})
	})
}
