package golang

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

var updateGolden = flag.Bool("update", false, "update golden files")

type goldenCase struct {
	name    string
	fixture string
	service model.ShapeID
	module  string
}

var goldenCases = []goldenCase{
	{
		name:    "weather",
		fixture: "../../testdata/weather.smithy",
		service: model.MustShapeID("example.weather#Weather"),
		module:  "example.com/weather",
	},
	{
		name:    "validators",
		fixture: "../../testdata/validators.smithy",
		service: model.MustShapeID("example.codegen#Example"),
		module:  "example.com/codegen",
	},
	{
		name:    "library",
		fixture: "../../testdata/library.smithy",
		service: model.MustShapeID("example.library#Library"),
		module:  "example.com/library",
	},
}

func TestGolden(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			m := loadFixture(t, tc.fixture)
			requireValid(t, m)
			files, diags, err := runGenerator(t, m, tc.service, tc.module, nil)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			for _, d := range diags {
				if d.Severity == validation.Danger {
					t.Errorf("unexpected DANGER diagnostic: %s", d)
				}
			}
			goldenDir := filepath.Join("testdata", "golden", tc.name)
			if *updateGolden {
				if err := os.RemoveAll(goldenDir); err != nil {
					t.Fatal(err)
				}
				writeFiles(t, goldenDir, files)
			}
			compareGolden(t, goldenDir, files)

			for _, file := range files {
				switch filepath.Base(file.Path) {
				case "client.go", "server.go", "service.go", "protocol.go":
					t.Errorf("Milestone 3 must not generate %s", file.Path)
				}
				if bytes.Contains(file.Content, []byte("net/http")) {
					t.Errorf("%s looks like protocol code", file.Path)
				}
			}

			dir := t.TempDir()
			writeFiles(t, dir, files)
			runGo(t, dir, "build", "./...")
			runGo(t, dir, "vet", "./...")
		})
	}
}

func TestGeneratedOutputIsDeterministic(t *testing.T) {
	m := loadFixture(t, "../../testdata/validators.smithy")
	first, _, err := runGenerator(t, m, model.MustShapeID("example.codegen#Example"), "example.com/codegen", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		next, _, err := runGenerator(t, m, model.MustShapeID("example.codegen#Example"), "example.com/codegen", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != len(next) {
			t.Fatalf("file count changed between runs: %d vs %d", len(first), len(next))
		}
		for j := range first {
			if first[j].Path != next[j].Path || !bytes.Equal(first[j].Content, next[j].Content) {
				t.Fatalf("output changed between runs for %s", first[j].Path)
			}
		}
	}
}

func TestGeneratedGoModHasNoDependencies(t *testing.T) {
	m := loadFixture(t, "../../testdata/weather.smithy")
	files, _, err := runGenerator(t, m, model.MustShapeID("example.weather#Weather"), "example.com/weather", nil)
	if err != nil {
		t.Fatal(err)
	}
	var gomod []byte
	for _, file := range files {
		if file.Path == "go.mod" {
			gomod = file.Content
		}
	}
	if gomod == nil {
		t.Fatal("generated module has no go.mod")
	}
	if strings.Contains(string(gomod), "require") {
		t.Errorf("go.mod has dependencies:\n%s", gomod)
	}
	if !strings.Contains(string(gomod), "module example.com/weather") {
		t.Errorf("go.mod module line missing:\n%s", gomod)
	}
}

func TestGeneratedValidatorBehavior(t *testing.T) {
	m := loadFixture(t, "../../testdata/validators.smithy")
	files, _, err := runGenerator(t, m, model.MustShapeID("example.codegen#Example"), "example.com/codegen", nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, files)
	// The generated module only contains library code; the test harness is
	// supplied by gosmithy and exercises the public API of package types.
	testFile := filepath.Join(dir, "types", "validate_behavior_test.go")
	if err := os.WriteFile(testFile, []byte(generatedValidatorTests), 0o644); err != nil {
		t.Fatal(err)
	}
	runGo(t, dir, "test", "./...")
}

func TestGenerateErrors(t *testing.T) {
	m := loadFixture(t, "../../testdata/weather.smithy")
	service := model.MustShapeID("example.weather#Weather")

	t.Run("missing module", func(t *testing.T) {
		_, _, err := runGenerator(t, m, service, "", nil)
		if err == nil || !strings.Contains(err.Error(), "module") {
			t.Fatalf("Generate error = %v, want module error", err)
		}
	})
	t.Run("missing service", func(t *testing.T) {
		_, _, err := runGenerator(t, m, model.MustShapeID("example.weather#Missing"), "example.com/x", nil)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("Generate error = %v, want not found", err)
		}
	})
	t.Run("not a service", func(t *testing.T) {
		_, _, err := runGenerator(t, m, model.MustShapeID("example.weather#CityId"), "example.com/x", nil)
		if err == nil || !strings.Contains(err.Error(), "not a service") {
			t.Fatalf("Generate error = %v, want not a service", err)
		}
	})
}

func TestUnknownOptionWarns(t *testing.T) {
	m := loadFixture(t, "../../testdata/weather.smithy")
	files, diags, err := runGenerator(t, m, model.MustShapeID("example.weather#Weather"), "example.com/weather", map[string]string{"future": "yes"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files generated")
	}
	found := false
	for _, d := range diags {
		if d.Severity == validation.Warning && strings.Contains(d.Message, "future") {
			found = true
		}
	}
	if !found {
		t.Errorf("diagnostics = %v, want unknown option warning", diags)
	}
}

func TestNamingCollisionIsDanger(t *testing.T) {
	m := assembleSources(t,
		`$version: "2.0"
namespace example.one
service Example {
    operations: [Op]
}
operation Op {
    input: In
}
structure In {
    a: Thing
    b: example.two#Thing
}
structure Thing {}
`,
		`$version: "2.0"
namespace example.two
structure Thing {}
`)
	_, diags, err := runGenerator(t, m, model.MustShapeID("example.one#Example"), "example.com/collision", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := false
	for _, d := range diags {
		if d.Severity == validation.Danger && strings.Contains(d.Message, "collides") {
			found = true
		}
	}
	if !found {
		t.Errorf("diagnostics = %v, want naming collision", diags)
	}
}

func TestServiceRenameApplies(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
service Example {
    operations: [Op]
    rename: {
        "example#Thing": "Record"
        "example#Thing$title": "name"
    }
}
operation Op {
    input: In
}
structure In {
    thing: Thing
}
structure Thing {
    title: String
}
`)
	files, diags, err := runGenerator(t, m, model.MustShapeID("example#Example"), "example.com/rename", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, d := range diags {
		if d.Severity == validation.Danger {
			t.Errorf("unexpected DANGER: %s", d)
		}
	}
	var typesFile string
	for _, file := range files {
		if file.Path == "types/types.go" {
			typesFile = string(file.Content)
		}
	}
	for _, want := range []string{"type Record struct", "Name *string", "Thing *Record"} {
		if !strings.Contains(typesFile, want) {
			t.Errorf("types.go missing %q:\n%s", want, typesFile)
		}
	}
	if strings.Contains(typesFile, "type Thing struct") {
		t.Errorf("rename did not remove original type name:\n%s", typesFile)
	}
}

func TestUnsupportedPatternIsError(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example
service Example {
    operations: [Op]
}
operation Op {
    input: In
}
structure In {
    @pattern("(?=x)")
    value: String
}
`)
	_, _, err := runGenerator(t, m, model.MustShapeID("example#Example"), "example.com/pattern", nil)
	if err == nil || !strings.Contains(err.Error(), "@pattern") {
		t.Fatalf("Generate error = %v, want @pattern error", err)
	}
}

// --- helpers ------------------------------------------------------------------

func loadFixture(t *testing.T, path string) *model.Model {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := idl.ParseWithOptions(path, string(src), idl.Options{Version: idl.V2_1, DefaultVersion: idl.V2_0})
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	m, err := model.Assemble([]*idl.File{file}, model.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func assembleSources(t *testing.T, sources ...string) *model.Model {
	t.Helper()
	files := make([]*idl.File, 0, len(sources))
	for i, src := range sources {
		name := fmt.Sprintf("test%d.smithy", i)
		file, err := idl.ParseWithOptions(name, src, idl.Options{Version: idl.V2_1, DefaultVersion: idl.V2_0})
		if err != nil {
			t.Fatalf("parsing source %d: %v", i, err)
		}
		files = append(files, file)
	}
	m, err := model.Assemble(files, model.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func requireValid(t *testing.T, m *model.Model) {
	t.Helper()
	for _, event := range validation.Validate(m) {
		if event.Severity == validation.Danger {
			t.Fatalf("fixture is invalid: %s [%s]", event.String(), event.ID)
		}
	}
}

func runGenerator(t *testing.T, m *model.Model, service model.ShapeID, module string, extra map[string]string) ([]codegen.File, []codegen.Diagnostic, error) {
	t.Helper()
	options := make(map[string]string, len(extra)+1)
	for key, value := range extra {
		options[key] = value
	}
	if module != "" {
		options["module"] = module
	}
	req := &codegen.Request{
		Model:   m,
		Service: service,
		Version: model.V2_0,
		Target:  codegen.Target{Name: GeneratorName, Generators: []string{GeneratorName}},
		Options: options,
	}
	resp, err := New().Generate(context.Background(), req)
	if err != nil {
		return nil, nil, err
	}
	if resp == nil {
		return nil, nil, fmt.Errorf("nil response")
	}
	return resp.Files, resp.Diagnostics, nil
}

func writeFiles(t *testing.T, dir string, files []codegen.File) {
	t.Helper()
	for _, file := range files {
		target := filepath.Join(dir, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, file.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func compareGolden(t *testing.T, goldenDir string, files []codegen.File) {
	t.Helper()
	generated := make(map[string][]byte, len(files))
	for _, file := range files {
		generated[file.Path] = file.Content
	}
	seen := make(map[string]bool, len(generated))
	err := filepath.WalkDir(goldenDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(goldenDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		want, ok := generated[rel]
		if !ok {
			t.Errorf("unexpected golden file %s", rel)
			return nil
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			t.Errorf("golden mismatch for %s; run go test ./codegen/golang -update", rel)
		}
		seen[rel] = true
		return nil
	})
	if os.IsNotExist(err) {
		t.Fatalf("golden directory %s does not exist; run go test ./codegen/golang -update", goldenDir)
	}
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for path := range generated {
		if !seen[path] {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("missing golden files (run with -update): %v", missing)
	}
}

func runGo(t *testing.T, dir string, args ...string) {
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

const generatedValidatorTests = `package types

import (
	"math/big"
	"strings"
	"testing"
	"time"

	smithy "example.com/codegen/internal/smithy"
)

func ptrString(v string) *string    { return &v }
func ptrInt32(v int32) *int32       { return &v }
func ptrKind(v ThingKind) *ThingKind { return &v }

func allErrorText(err error) string {
	switch e := err.(type) {
	case smithy.ValidationErrors:
		var b strings.Builder
		for _, item := range e {
			b.WriteString(item.Error())
			b.WriteByte('\n')
		}
		return b.String()
	case *smithy.ValidationErrors:
		return allErrorText(*e)
	default:
		return err.Error()
	}
}

func failUnlessContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(allErrorText(err), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

func validThing() *Thing {
	return &Thing{
		Id:    ptrString("thing"),
		Title: ptrString("A thing"),
		Kind:  ptrKind(ThingKindWidget),
	}
}

func TestRequiredUsesNilNotEmpty(t *testing.T) {
	var in GetThingInput
	failUnlessContains(t, in.Validate(), "id: is required")

	in.Id = ptrString("")
	err := in.Validate()
	failUnlessContains(t, err, "id: must be at least 2 characters")
	if strings.Contains(err.Error(), "id: is required") {
		t.Fatalf("empty string reported as missing: %v", err)
	}
}

func TestLengthRangePatternBlob(t *testing.T) {
	thing := validThing()
	thing.Id = ptrString("UPPER")
	failUnlessContains(t, thing.Validate(), "id: must match pattern")

	thing = validThing()
	thing.Percentage = ptrInt32(200)
	failUnlessContains(t, thing.Validate(), "percentage: must be less than or equal to 100")

	thing = validThing()
	thing.Percentage = ptrInt32(-1)
	failUnlessContains(t, thing.Validate(), "percentage: must be greater than or equal to 1")

	thing = validThing()
	thing.Title = ptrString("")
	failUnlessContains(t, thing.Validate(), "title: must be at least 1 character")

	thing = validThing()
	thing.Title = ptrString("é")
	if err := thing.Validate(); err != nil {
		t.Fatalf("one-rune title failed validation: %v", err)
	}

	thing = validThing()
	badSize := Size("BAD")
	thing.Size = &badSize
	failUnlessContains(t, thing.Validate(), "size: must match pattern")

	thing = validThing()
	thing.Payload = []byte{}
	failUnlessContains(t, thing.Validate(), "payload: must be at least 1 byte")
}

func TestCollectionsAndNested(t *testing.T) {
	thing := validThing()
	thing.Tags = Tags{"ok", "BAD!"}
	failUnlessContains(t, thing.Validate(), "tags[1]: must match pattern")

	thing = validThing()
	thing.Scores = Scores{1, 1}
	failUnlessContains(t, thing.Validate(), "scores[0]: must contain unique items")

	thing = validThing()
	thing.Scores = Scores{-1}
	failUnlessContains(t, thing.Validate(), "scores[0]: must be greater than or equal to 0")

	thing = validThing()
	thing.Tags = Tags{"a", "b", "c", "d", "e"}
	failUnlessContains(t, thing.Validate(), "tags: must be at most 4 items")

	thing = validThing()
	thing.Nested = &Nested{}
	failUnlessContains(t, thing.Validate(), "nested.value: is required")

	thing = validThing()
	bad := "BAD!"
	thing.OptionalTags = OptionalTags{nil, &bad}
	err := thing.Validate()
	failUnlessContains(t, err, "optionalTags[1]: must match pattern")
	if strings.Contains(err.Error(), "optionalTags[0]") {
		t.Fatalf("nil sparse element was validated: %v", err)
	}
}

func TestNestedCollectionPaths(t *testing.T) {
	out := &GetThingOutput{
		Related: ThingList{{}},
		Index:   ThingMap{"a": {}},
	}
	err := out.Validate()
	failUnlessContains(t, err, "related[0].id: is required")
	failUnlessContains(t, err, "index[a].id: is required")
}

func TestMemberOverridesTargetConstraints(t *testing.T) {
	in := &ListThingsInput{Ids: ThingIdList{"a"}}
	failUnlessContains(t, in.Validate(), "ids[0]: must be at least 2 characters")
}

func TestErrorTypes(t *testing.T) {
	notFound := &ThingNotFound{Message: ptrString("missing")}
	var err error = notFound
	if got := err.Error(); got != "missing" {
		t.Errorf("Error() = %q, want missing", got)
	}
	if got := notFound.ErrorCode(); got != "ThingNotFound" {
		t.Errorf("ErrorCode() = %q", got)
	}
	if got := notFound.ErrorFault(); got != "client" {
		t.Errorf("ErrorFault() = %q", got)
	}
	if got := (&ThingNotFound{}).Error(); got != "ThingNotFound" {
		t.Errorf("empty Error() = %q", got)
	}
	if got := (&ValidationFailure{}).ErrorFault(); got != "server" {
		t.Errorf("server ErrorFault() = %q", got)
	}
}

func TestScalarTypes(t *testing.T) {
	var s Scalars
	b := true
	i8 := int8(1)
	i16 := int16(1)
	i32 := int32(1)
	i64 := int64(1)
	f32 := float32(1)
	f64 := float64(1)
	now := time.Now()
	s = Scalars{
		AString:     ptrString("x"),
		ABoolean:    &b,
		ABlob:       []byte("x"),
		AByte:       &i8,
		AShort:      &i16,
		AnInteger:   &i32,
		ALong:       &i64,
		AFloat:      &f32,
		ADouble:     &f64,
		ABigInteger: big.NewInt(1),
		ABigDecimal: big.NewFloat(1),
		ATimestamp:  &now,
		ADocument:   map[string]any{"a": 1},
		OptionalMap: OptionalAttributes{"key": nil},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Scalars.Validate() = %v", err)
	}
}

func TestEnumsAndUnions(t *testing.T) {
	if got := ThingKindWidget.Values(); len(got) != 2 {
		t.Errorf("ThingKind values = %v", got)
	}
	if got := PriorityLow.Values(); len(got) != 2 {
		t.Errorf("Priority values = %v", got)
	}
	if got := SizeSmall.Values(); len(got) != 2 {
		t.Errorf("Size values = %v", got)
	}
	var shape Shape = &ShapeCircle{Value: &Circle{}}
	if _, ok := shape.(*ShapeCircle); !ok {
		t.Fatalf("shape has unexpected type %T", shape)
	}
	var _ Shape = (*ShapeSquare)(nil)
	var _ Shape = (*ShapeLabel)(nil)
}
`
