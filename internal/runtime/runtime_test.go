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
}
