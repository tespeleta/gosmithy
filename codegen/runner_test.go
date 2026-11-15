package codegen

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/validation"
)

func fakeGenerator(files []File, diags ...Diagnostic) Generator {
	return GeneratorFunc(func(context.Context, *Request) (*Response, error) {
		return &Response{Files: files, Diagnostics: diags}, nil
	})
}

func TestRunnerMergesAndWrites(t *testing.T) {
	out := t.TempDir()
	var logs bytes.Buffer
	r := &Runner{OutDir: out, Output: &logs}
	req := &Request{}

	result, err := r.Run(context.Background(), req, []Generator{
		fakeGenerator([]File{
			{Path: "b.txt", Content: []byte("second\n")},
			{Path: "types/types.go", Content: []byte("package types\n")},
		}, Diagnostic{Severity: validation.Warning, Message: "heads up", Path: "x.smithy", Line: 2, Column: 3}),
		fakeGenerator([]File{
			{Path: "a.txt", Content: []byte("first\n"), Mode: 0o600},
		}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := len(result.Files), 3; got != want {
		t.Fatalf("files = %d, want %d", got, want)
	}
	if got, want := strings.Join(result.Written, ","), "a.txt,b.txt,types/types.go"; got != want {
		t.Errorf("written = %q, want %q", got, want)
	}
	if got, want := len(result.Diagnostics), 1; got != want {
		t.Errorf("diagnostics = %d, want %d", got, want)
	}
	// Files are sorted by path.
	if result.Files[0].Path != "a.txt" {
		t.Errorf("first file = %q, want a.txt", result.Files[0].Path)
	}
	data, err := os.ReadFile(filepath.Join(out, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first\n" {
		t.Errorf("a.txt = %q", data)
	}
	info, err := os.Stat(filepath.Join(out, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("a.txt mode = %o, want 600", info.Mode().Perm())
	}
	for _, want := range []string{"1 diagnostic(s)", "3 file(s) written", "WARNING"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("summary missing %q:\n%s", want, logs.String())
		}
	}
}

func TestRunnerAllowsIdenticalDuplicatePaths(t *testing.T) {
	out := t.TempDir()
	r := &Runner{OutDir: out}
	file := File{Path: "same.txt", Content: []byte("same")}
	result, err := r.Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{file}),
		fakeGenerator([]File{file}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := len(result.Files), 1; got != want {
		t.Fatalf("files = %d, want %d", got, want)
	}
	if got, want := len(result.Skipped), 1; got != want || result.Skipped[0] != "same.txt" {
		t.Errorf("skipped = %v, want [same.txt]", result.Skipped)
	}
	if got, want := len(result.Written), 1; got != want {
		t.Errorf("written = %v, want one file", result.Written)
	}
}

func TestRunnerRejectsConflictingPaths(t *testing.T) {
	out := t.TempDir()
	r := &Runner{OutDir: out}
	_, err := r.Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{{Path: "same.txt", Content: []byte("one")}}),
		fakeGenerator([]File{{Path: "same.txt", Content: []byte("two")}}),
	})
	if err == nil || !strings.Contains(err.Error(), "conflicting output") {
		t.Fatalf("Run error = %v, want conflicting output", err)
	}
	assertDirEmpty(t, out)
}

func TestRunnerRejectsConflictingModes(t *testing.T) {
	out := t.TempDir()
	r := &Runner{OutDir: out}
	_, err := r.Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{{Path: "same.txt", Content: []byte("same"), Mode: 0o644}}),
		fakeGenerator([]File{{Path: "same.txt", Content: []byte("same"), Mode: 0o600}}),
	})
	if err == nil || !strings.Contains(err.Error(), "conflicting output") {
		t.Fatalf("Run error = %v, want conflicting output", err)
	}
	assertDirEmpty(t, out)
}

func TestRunnerRejectsEscapingPaths(t *testing.T) {
	paths := []string{"", ".", "..", "../x", "a/../../x", "/abs", "a\\b"}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			out := t.TempDir()
			r := &Runner{OutDir: out}
			_, err := r.Run(context.Background(), &Request{}, []Generator{
				fakeGenerator([]File{{Path: p, Content: []byte("x")}}),
			})
			if err == nil {
				t.Fatalf("Run accepted path %q", p)
			}
			assertDirEmpty(t, out)
		})
	}
}

func TestRunnerDangerDiagnosticsAbort(t *testing.T) {
	out := t.TempDir()
	var logs bytes.Buffer
	r := &Runner{OutDir: out, Output: &logs}
	result, err := r.Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{{Path: "a.txt", Content: []byte("a")}},
			Diagnostic{Severity: validation.Danger, Message: "bad name", Path: "a.smithy", Line: 1, Column: 2}),
	})
	if err == nil || !strings.Contains(err.Error(), "DANGER") {
		t.Fatalf("Run error = %v, want DANGER", err)
	}
	if result == nil || len(result.Files) != 1 {
		t.Errorf("result = %+v, want planned file", result)
	}
	if len(result.Written) != 0 {
		t.Errorf("written = %v, want none", result.Written)
	}
	assertDirEmpty(t, out)
	if !strings.Contains(logs.String(), "a.smithy:1:2: DANGER: bad name") {
		t.Errorf("missing diagnostic log:\n%s", logs.String())
	}
}

func TestRunnerGeneratorErrorWritesNothing(t *testing.T) {
	out := t.TempDir()
	r := &Runner{OutDir: out}
	failing := GeneratorFunc(func(context.Context, *Request) (*Response, error) {
		return nil, errors.New("boom")
	})
	_, err := r.Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{{Path: "a.txt", Content: []byte("a")}}),
		failing,
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Run error = %v, want boom", err)
	}
	assertDirEmpty(t, out)
}

func TestRunnerDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "does-not-exist")
	var logs bytes.Buffer
	r := &Runner{OutDir: out, DryRun: true, Output: &logs}
	result, err := r.Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{{Path: "a.txt", Content: []byte("a")}}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Written) != 0 || !result.DryRun {
		t.Errorf("result = %+v", result)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("dry run created output directory")
	}
	if !strings.Contains(logs.String(), "1 file(s) planned") {
		t.Errorf("summary = %q", logs.String())
	}
}

func TestRunnerRequiresOutDir(t *testing.T) {
	r := &Runner{}
	_, err := r.Run(context.Background(), &Request{}, []Generator{fakeGenerator(nil)})
	if err == nil || !strings.Contains(err.Error(), "output directory") {
		t.Fatalf("Run error = %v, want output directory error", err)
	}
}

func TestRunnerRequiresGenerators(t *testing.T) {
	r := &Runner{OutDir: t.TempDir()}
	_, err := r.Run(context.Background(), &Request{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no generators") {
		t.Fatalf("Run error = %v, want no generators", err)
	}
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("output directory is not empty: %v", names)
	}
}

// namedFake is a generator that reports a name in errors.
type namedFake struct {
	name string
	Generator
}

func (n namedFake) Name() string { return n.name }

func TestRunnerNamesGeneratorInErrors(t *testing.T) {
	failing := namedFake{
		name: "fancy",
		Generator: GeneratorFunc(func(context.Context, *Request) (*Response, error) {
			return nil, errors.New("boom")
		}),
	}
	r := &Runner{OutDir: t.TempDir()}
	_, err := r.Run(context.Background(), &Request{}, []Generator{failing})
	if err == nil || !strings.Contains(err.Error(), "codegen: fancy: boom") {
		t.Fatalf("Run error = %v, want the generator name", err)
	}
}

func TestRunnerCleanRemovesPreviousOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gen")
	first := &Runner{OutDir: out}
	if _, err := first.Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{
			{Path: "keep.txt", Content: []byte("old")},
			{Path: "old/drop.txt", Content: []byte("old")},
		}),
	}); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	// Anything in a generated directory is owned by the tool, including files
	// this run did not create.
	if err := os.WriteFile(filepath.Join(out, "notes.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := &Runner{OutDir: out, Clean: true}
	result, err := second.Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{{Path: "fresh.txt", Content: []byte("new")}}),
	})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	for _, gone := range []string{"keep.txt", "old/drop.txt", "notes.txt", "old"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(gone))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s survived --clean", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "fresh.txt")); err != nil {
		t.Errorf("fresh.txt missing: %v", err)
	}
	if got, want := strings.Join(result.Written, ","), "fresh.txt"; got != want {
		t.Errorf("written = %q, want %q", got, want)
	}
}

func TestRunnerCleanDryRunKeepsOutput(t *testing.T) {
	out := t.TempDir()
	if _, err := (&Runner{OutDir: out}).Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{{Path: "a.txt", Content: []byte("a")}}),
	}); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	result, err := (&Runner{OutDir: out, DryRun: true, Clean: true}).Run(context.Background(), &Request{}, []Generator{
		fakeGenerator([]File{{Path: "b.txt", Content: []byte("b")}}),
	})
	if err != nil {
		t.Fatalf("dry Run: %v", err)
	}
	if len(result.Written) != 0 {
		t.Errorf("dry run wrote %v", result.Written)
	}
	if _, err := os.Stat(filepath.Join(out, "a.txt")); err != nil {
		t.Errorf("dry run with Clean removed an output: %v", err)
	}
}

func TestCheckCleanableRefusesOwnedParents(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkCleanable(cwd); err == nil || !strings.Contains(err.Error(), "working directory") {
		t.Errorf("checkCleanable(cwd) = %v, want refusal", err)
	}
	if err := checkCleanable(filepath.Dir(cwd)); err == nil || !strings.Contains(err.Error(), "working directory") {
		t.Errorf("checkCleanable(parent) = %v, want refusal", err)
	}
	if err := checkCleanable(string(os.PathSeparator)); err == nil {
		t.Errorf("checkCleanable(root) = %v, want refusal", err)
	}
}

func TestRunnerDedupesIdenticalDiagnostics(t *testing.T) {
	diag := Diagnostic{Severity: validation.Warning, Message: "same", Path: "a.smithy", Line: 1, Column: 2}
	result, err := (&Runner{OutDir: t.TempDir()}).Run(context.Background(), &Request{}, []Generator{
		fakeGenerator(nil, diag),
		fakeGenerator(nil, diag),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(result.Diagnostics), 1; got != want {
		t.Errorf("diagnostics = %d, want %d: %+v", got, want, result.Diagnostics)
	}
}
