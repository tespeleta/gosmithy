package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func weatherPath() string { return filepath.Join("..", "..", "testdata", "weather.smithy") }

func TestGenerateWeather(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gen")
	output, err := captureStdout(t, func() error {
		return run([]string{
			"generate",
			"--service", "example.weather#Weather",
			"--generator", "go",
			"--module", "example.com/weather",
			"--out", out,
			weatherPath(),
		})
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(output, "7 file(s) written") {
		t.Errorf("output = %q, want file summary", output)
	}
	for _, name := range []string{
		"go.mod",
		"internal/smithy/validate.go",
		"internal/smithy/json.go",
		"types/types.go",
		"types/enums.go",
		"types/unions.go",
		"types/validate.go",
	} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(name))); err != nil {
			t.Errorf("missing generated file %s: %v", name, err)
		}
	}
	gomod, err := os.ReadFile(filepath.Join(out, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gomod), "module example.com/weather") {
		t.Errorf("go.mod = %q", gomod)
	}
	types, err := os.ReadFile(filepath.Join(out, "types", "types.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(types), "type GetCityInput struct") {
		t.Errorf("types.go missing GetCityInput:\n%s", types)
	}
}

func TestGenerateListGenerators(t *testing.T) {
	output, err := captureStdout(t, func() error {
		return run([]string{"generate", "--list-generators"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output) != "go" {
		t.Errorf("list output = %q, want go", output)
	}
}

func TestGenerateRequiresFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "service",
			args: []string{"generate", "--module", "example.com/x", "--out", t.TempDir(), weatherPath()},
			want: "--service",
		},
		{
			name: "module",
			args: []string{"generate", "--service", "example.weather#Weather", "--out", t.TempDir(), weatherPath()},
			want: "--module",
		},
		{
			name: "out",
			args: []string{"generate", "--service", "example.weather#Weather", "--module", "example.com/x", weatherPath()},
			want: "--out",
		},
		{
			name: "files",
			args: []string{"generate", "--service", "example.weather#Weather", "--module", "example.com/x", "--out", t.TempDir()},
			want: "at least one file",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := captureStdout(t, func() error { return run(tc.args) })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestGenerateUnknownGenerator(t *testing.T) {
	_, err := captureStdout(t, func() error {
		return run([]string{
			"generate",
			"--service", "example.weather#Weather",
			"--generator", "missing",
			"--module", "example.com/x",
			"--out", t.TempDir(),
			weatherPath(),
		})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown generator") {
		t.Fatalf("error = %v, want unknown generator", err)
	}
}

func TestGenerateOptionWarning(t *testing.T) {
	output, err := captureStdout(t, func() error {
		return run([]string{
			"generate",
			"--service", "example.weather#Weather",
			"--module", "example.com/x",
			"--out", t.TempDir(),
			"--opt", "future=yes",
			weatherPath(),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "1 diagnostic(s)") {
		t.Errorf("output = %q, want diagnostic summary", output)
	}
}

func TestGenerateInvalidOption(t *testing.T) {
	_, err := captureStdout(t, func() error {
		return run([]string{
			"generate",
			"--service", "example.weather#Weather",
			"--module", "example.com/x",
			"--out", t.TempDir(),
			"--opt", "novalue",
			weatherPath(),
		})
	})
	if err == nil || !strings.Contains(err.Error(), "key=value") {
		t.Fatalf("error = %v, want key=value", err)
	}
}

func TestGenerateLibraryHTTP(t *testing.T) {
	library := filepath.Join("..", "..", "testdata", "library.smithy")
	out := filepath.Join(t.TempDir(), "gen")
	output, err := captureStdout(t, func() error {
		return run([]string{
			"generate",
			"--service", "example.library#Library",
			"--generator", "go",
			"--protocol", "gosmithy.protocols#httpJson",
			"--module", "example.com/library",
			"--out", out,
			library,
		})
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(output, "written") {
		t.Errorf("output = %q, want file summary", output)
	}
	for _, name := range []string{
		"go.mod",
		"service.go",
		"client.go",
		"server.go",
		"protocol.go",
		"internal/smithy/http.go",
		"internal/smithy/json.go",
		"types/types.go",
	} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(name))); err != nil {
			t.Errorf("missing generated file %s: %v", name, err)
		}
	}
	runGoCommand(t, out, "build", "./...")
	runGoCommand(t, out, "test", "./...")
}

func TestGenerateUnknownProtocol(t *testing.T) {
	library := filepath.Join("..", "..", "testdata", "library.smithy")
	_, err := captureStdout(t, func() error {
		return run([]string{
			"generate",
			"--service", "example.library#Library",
			"--protocol", "aws#restJson1",
			"--module", "example.com/library",
			"--out", t.TempDir(),
			library,
		})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown protocol") {
		t.Fatalf("error = %v, want unknown protocol", err)
	}
}

func TestGenerateModelOnlyHasNoProtocolCode(t *testing.T) {
	library := filepath.Join("..", "..", "testdata", "library.smithy")
	out := filepath.Join(t.TempDir(), "gen")
	if _, err := captureStdout(t, func() error {
		return run([]string{
			"generate",
			"--service", "example.library#Library",
			"--module", "example.com/library",
			"--out", out,
			library,
		})
	}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, name := range []string{"client.go", "server.go", "service.go", "protocol.go", "internal/smithy/http.go"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(name))); err == nil {
			t.Errorf("model-only generation wrote protocol file %s", name)
		}
	}
}

func runGoCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	env := os.Environ()
	if os.Getenv("GOCACHE") == "" {
		env = append(env, "GOCACHE="+t.TempDir())
	}
	env = append(env, "GOWORK=off")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}
