package runtime

import (
	"bytes"
	"testing"
)

func TestFiles(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	content, ok := files["internal/smithy/validate.go"]
	if !ok {
		t.Fatalf("runtime files = %v, want internal/smithy/validate.go", files)
	}
	for _, want := range []string{"package smithy", "type ValidationErrors", "func (e *ValidationErrors) AddNested"} {
		if !bytes.Contains(content, []byte(want)) {
			t.Errorf("runtime source missing %q", want)
		}
	}
	jsonContent, ok := files["internal/smithy/json.go"]
	if !ok {
		t.Fatalf("runtime files = %v, want internal/smithy/json.go", files)
	}
	for _, want := range []string{"func FormatTimestamp", "func ParseTimestamp", "TimestampEpochMilliseconds"} {
		if !bytes.Contains(jsonContent, []byte(want)) {
			t.Errorf("json runtime source missing %q", want)
		}
	}
	if _, ok := files["internal/smithy/http.go"]; ok {
		t.Error("model-only runtime must not include the HTTP protocol helpers")
	}
	if _, ok := files["internal/smithy/relaxed.go"]; ok {
		t.Error("model-only runtime must not include the relaxed numeric helpers by default")
	}
}

func TestRelaxedFiles(t *testing.T) {
	files, err := RelaxedFiles()
	if err != nil {
		t.Fatal(err)
	}
	content, ok := files["internal/smithy/relaxed.go"]
	if !ok {
		t.Fatalf("relaxed files = %v, want internal/smithy/relaxed.go", files)
	}
	for _, want := range []string{"func UnmarshalRelaxedInt8", "func UnmarshalRelaxedFloat64", "func UnmarshalRelaxedBigInteger"} {
		if !bytes.Contains(content, []byte(want)) {
			t.Errorf("relaxed runtime source missing %q", want)
		}
	}
}

func TestProtocolFiles(t *testing.T) {
	files, err := FilesFor(true)
	if err != nil {
		t.Fatal(err)
	}
	content, ok := files["internal/smithy/http.go"]
	if !ok {
		t.Fatalf("runtime files = %v, want internal/smithy/http.go", files)
	}
	for _, want := range []string{"type ProtocolError", "func NewProtocolError", "func ReadBody", "func ValidationErrorList"} {
		if !bytes.Contains(content, []byte(want)) {
			t.Errorf("http runtime source missing %q", want)
		}
	}
}
