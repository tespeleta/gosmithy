package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	output := <-done
	_ = r.Close()
	return output, runErr
}

func TestValidateExitCodes(t *testing.T) {
	weather := filepath.Join("..", "..", "testdata", "weather.smithy")
	broken := filepath.Join("..", "..", "testdata", "broken.smithy")

	output, err := captureStdout(t, func() error {
		return run([]string{"validate", weather})
	})
	if err != nil {
		t.Fatalf("validate weather: %v", err)
	}
	if output != "" {
		t.Errorf("valid model produced output: %q", output)
	}

	output, err = captureStdout(t, func() error {
		return run([]string{"validate", broken})
	})
	var exit *exitError
	if !errors.As(err, &exit) || exit.code != 1 {
		t.Fatalf("validate broken error = %v, want exit 1", err)
	}
	if !strings.Contains(output, "DANGER") || !strings.Contains(output, "MissingShape") {
		t.Errorf("broken model output = %q", output)
	}
}

func TestValidateJSON(t *testing.T) {
	broken := filepath.Join("..", "..", "testdata", "broken.smithy")
	output, err := captureStdout(t, func() error {
		return run([]string{"validate", "--json", broken})
	})
	var exit *exitError
	if !errors.As(err, &exit) || exit.code != 1 {
		t.Fatalf("validate error = %v", err)
	}
	if !strings.Contains(output, `"id": "Model.UnresolvedShape"`) {
		t.Errorf("JSON output = %q", output)
	}
	if !strings.Contains(output, `"severity": "DANGER"`) {
		t.Errorf("JSON output = %q", output)
	}
}

func TestValidateSeverityFilter(t *testing.T) {
	// The broken model only has a DANGER event, so raising the threshold to
	// danger keeps it, while the JSON flag is orthogonal.
	broken := filepath.Join("..", "..", "testdata", "broken.smithy")
	_, err := captureStdout(t, func() error {
		return run([]string{"validate", "-min-severity", "danger", broken})
	})
	var exit *exitError
	if !errors.As(err, &exit) || exit.code != 1 {
		t.Fatalf("validate error = %v", err)
	}
}

func TestModelJSONIsDeterministic(t *testing.T) {
	weather := filepath.Join("..", "..", "testdata", "weather.smithy")
	first, err := captureStdout(t, func() error {
		return run([]string{"model", "--json", "-prelude=false", weather})
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		next, err := captureStdout(t, func() error {
			return run([]string{"model", "--json", "-prelude=false", weather})
		})
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("model JSON changed between runs")
		}
	}
	if !strings.Contains(first, `"example.weather#Weather"`) {
		t.Errorf("model JSON missing service: %.200s", first)
	}
	var decoded struct {
		Shapes map[string]any `json:"shapes"`
	}
	if err := json.Unmarshal([]byte(first), &decoded); err != nil {
		t.Fatalf("model JSON is invalid: %v", err)
	}
	for id := range decoded.Shapes {
		if strings.HasPrefix(id, "smithy.api#") {
			t.Errorf("model JSON includes prelude shape %s despite -prelude=false", id)
		}
	}
}

func TestParseFilesDefaultsToVersion2(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "loader", "valid", "main-test.smithy")
	files, err := parseFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("parsed %d files", len(files))
	}
	if files[0].Version != idl.V2_0 {
		t.Errorf("default version = %s, want 2.0", files[0].Version)
	}
}
