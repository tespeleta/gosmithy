package codegen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tespeleta/gosmithy/validation"
)

// Runner merges generator responses and writes the output tree.
//
// All generators are called before anything is written, and DANGER
// diagnostics abort the run. This gives code generation all-or-nothing
// semantics at the generator level.
type Runner struct {
	// OutDir is the output directory. It is required unless DryRun is set.
	OutDir string
	// DryRun plans the output without writing it.
	DryRun bool
	// Clean removes the output directory before writing. Generated directories
	// are owned by the tool, so no state from a previous run is kept.
	Clean bool
	// Output receives diagnostics and a stable summary. A nil Output disables
	// printing.
	Output io.Writer
}

// RunResult describes what a run planned and wrote.
type RunResult struct {
	// Files is the merged, sorted set of planned files.
	Files []File
	// Written lists the paths written to disk, in sorted order. It is empty
	// for a dry run.
	Written []string
	// Skipped lists duplicate paths whose contents were identical.
	Skipped []string
	// Diagnostics is the sorted set of generator diagnostics.
	Diagnostics []Diagnostic
	// DryRun reports whether this was a dry run.
	DryRun bool
}

// Run calls every generator with the same request, merges the responses,
// detects path conflicts, and writes the result.
func (r *Runner) Run(ctx context.Context, req *Request, gens []Generator) (*RunResult, error) {
	if len(gens) == 0 {
		return nil, errors.New("codegen: no generators selected")
	}
	if r.OutDir == "" && !r.DryRun {
		return nil, errors.New("codegen: output directory is required")
	}

	result := &RunResult{DryRun: r.DryRun}
	merged := make(map[string]File)
	seen := make(map[string]bool)
	var paths []string

	for i, gen := range gens {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if gen == nil {
			return result, fmt.Errorf("codegen: %s is nil", generatorName(nil, i))
		}
		resp, err := gen.Generate(ctx, req)
		if err != nil {
			return result, fmt.Errorf("codegen: %s: %w", generatorName(gen, i), err)
		}
		if resp == nil {
			continue
		}
		result.Diagnostics = append(result.Diagnostics, resp.Diagnostics...)
		for _, file := range resp.Files {
			clean, err := cleanRelativePath(file.Path)
			if err != nil {
				return result, fmt.Errorf("codegen: %s: %w", generatorName(gen, i), err)
			}
			file.Path = clean
			if existing, ok := merged[clean]; ok {
				if !bytes.Equal(existing.Content, file.Content) || normalizeMode(existing.Mode) != normalizeMode(file.Mode) {
					return result, fmt.Errorf("codegen: conflicting output for path %q", clean)
				}
				if !seen[clean] {
					result.Skipped = append(result.Skipped, clean)
					seen[clean] = true
				}
				continue
			}
			merged[clean] = file
			paths = append(paths, clean)
		}
	}

	sort.Strings(paths)
	sort.SliceStable(result.Diagnostics, func(i, j int) bool {
		return diagnosticLess(result.Diagnostics[i], result.Diagnostics[j])
	})
	result.Diagnostics = dedupeDiagnostics(result.Diagnostics)
	result.Files = make([]File, 0, len(paths))
	for _, p := range paths {
		result.Files = append(result.Files, merged[p])
	}

	r.printDiagnostics(result.Diagnostics)
	if hasDanger(result.Diagnostics) {
		r.printSummary(result)
		return result, fmt.Errorf("codegen: generation failed: %s", countDanger(result.Diagnostics))
	}

	if r.DryRun {
		r.printSummary(result)
		return result, nil
	}

	outAbs, err := filepath.Abs(r.OutDir)
	if err != nil {
		return result, fmt.Errorf("codegen: resolving output directory: %w", err)
	}
	if r.Clean {
		if err := cleanOutputDir(outAbs); err != nil {
			return result, err
		}
	}
	for _, file := range result.Files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		target, err := safeOutputPath(outAbs, file.Path)
		if err != nil {
			return result, err
		}
		if err := writeFileAtomic(target, file.Content, normalizeMode(file.Mode)); err != nil {
			return result, fmt.Errorf("codegen: writing %s: %w", file.Path, err)
		}
		result.Written = append(result.Written, file.Path)
	}
	sort.Strings(result.Written)

	r.printSummary(result)
	return result, nil
}

// cleanOutputDir removes the output directory before a run so that only files
// from this run remain.
func cleanOutputDir(outAbs string) error {
	if err := checkCleanable(outAbs); err != nil {
		return err
	}
	if err := os.RemoveAll(outAbs); err != nil {
		return fmt.Errorf("codegen: cleaning %s: %w", outAbs, err)
	}
	return nil
}

// checkCleanable rejects output directories the tool must never delete: the
// filesystem root, the working directory, and its ancestors. A generated
// directory never contains the working directory.
func checkCleanable(outAbs string) error {
	if filepath.Dir(outAbs) == outAbs {
		return fmt.Errorf("codegen: refusing to clean the filesystem root %q", outAbs)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("codegen: resolving the working directory: %w", err)
	}
	if cwd == outAbs || strings.HasPrefix(cwd, outAbs+string(filepath.Separator)) {
		return fmt.Errorf("codegen: refusing to clean %q: it contains the working directory", outAbs)
	}
	return nil
}

// generatorName identifies a generator in error messages, preferring its own
// name over its position in the run.
func generatorName(gen Generator, index int) string {
	if named, ok := gen.(NamedGenerator); ok {
		if name := named.Name(); name != "" {
			return name
		}
	}
	return fmt.Sprintf("generator %d", index)
}

func (r *Runner) printDiagnostics(diags []Diagnostic) {
	if r.Output == nil {
		return
	}
	for _, d := range diags {
		fmt.Fprintln(r.Output, d.String())
	}
}

func (r *Runner) printSummary(result *RunResult) {
	if r.Output == nil {
		return
	}
	count := len(result.Written)
	verb := "written"
	if result.DryRun {
		count = len(result.Files)
		verb = "planned"
	}
	line := fmt.Sprintf("%d file(s) %s, %d skipped, %d diagnostic(s)",
		count, verb, len(result.Skipped), len(result.Diagnostics))
	fmt.Fprintln(r.Output, line)
}

func hasDanger(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == validation.Danger {
			return true
		}
	}
	return false
}

func countDanger(diags []Diagnostic) string {
	n := 0
	for _, d := range diags {
		if d.Severity == validation.Danger {
			n++
		}
	}
	if n == 1 {
		return "1 DANGER diagnostic"
	}
	return fmt.Sprintf("%d DANGER diagnostics", n)
}

func dedupeDiagnostics(diags []Diagnostic) []Diagnostic {
	if len(diags) < 2 {
		return diags
	}
	out := diags[:1]
	for _, diag := range diags[1:] {
		previous := out[len(out)-1]
		if previous.Severity == diag.Severity && previous.Message == diag.Message &&
			previous.Path == diag.Path && previous.Line == diag.Line && previous.Column == diag.Column {
			continue
		}
		out = append(out, diag)
	}
	return out
}

func diagnosticLess(a, b Diagnostic) bool {
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	if a.Column != b.Column {
		return a.Column < b.Column
	}
	if a.Severity != b.Severity {
		return a.Severity < b.Severity
	}
	return a.Message < b.Message
}

func normalizeMode(mode fs.FileMode) fs.FileMode {
	if mode == 0 {
		return 0o644
	}
	return mode.Perm()
}

// cleanRelativePath validates a generator-provided path and returns its
// canonical slash-separated form.
func cleanRelativePath(p string) (string, error) {
	if p == "" {
		return "", errors.New("empty output path")
	}
	if strings.ContainsRune(p, '\\') {
		return "", fmt.Errorf("output path %q must use forward slashes", p)
	}
	if path.IsAbs(p) || filepath.IsAbs(p) || filepath.VolumeName(p) != "" {
		return "", fmt.Errorf("output path %q must be relative", p)
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." {
			return "", fmt.Errorf("output path %q must not contain '..'", p)
		}
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("output path %q escapes the output directory", p)
	}
	return clean, nil
}

// safeOutputPath joins a cleaned relative path to an absolute output directory
// and verifies the result remains inside that directory.
func safeOutputPath(outAbs, rel string) (string, error) {
	target := filepath.Join(outAbs, filepath.FromSlash(rel))
	inside, err := filepath.Rel(outAbs, target)
	if err != nil {
		return "", fmt.Errorf("codegen: resolving output path %q: %w", rel, err)
	}
	if inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("codegen: output path %q escapes the output directory", rel)
	}
	return target, nil
}

func writeFileAtomic(target string, content []byte, mode fs.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".gosmithy-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup; Rename removes the temporary name on success.
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}
