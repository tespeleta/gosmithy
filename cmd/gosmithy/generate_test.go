package main

import (
	"os"
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
	if !strings.Contains(output, "6 file(s) written") {
		t.Errorf("output = %q, want file summary", output)
	}
	for _, name := range []string{
		"go.mod",
		"internal/smithy/validate.go",
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
