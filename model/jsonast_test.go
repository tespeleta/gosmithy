package model_test

import (
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

func parseJSONASTFixture(t *testing.T, path string) *idl.File {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	file, err := idl.ParseJSONAST(filepath.Base(path), src)
	if err != nil {
		t.Fatalf("parsing JSON AST %s: %v", path, err)
	}
	return file
}

func assembleJSONFixture(t *testing.T, files ...*idl.File) *model.Model {
	t.Helper()
	m, err := model.Assemble(files, model.Options{})
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	return m
}

// TestJSONASTFixturesMatchIDL loads every upstream valid JSON AST fixture and
// asserts that it produces the same semantic model as the matching IDL
// fixture. Both sides use their upstream shared fixture.
func TestJSONASTFixturesMatchIDL(t *testing.T) {
	root := filepath.Join(testdataRoot, "loader", "valid")
	sharedIDL := parseFile(t, filepath.Join(testdataRoot, "shared.smithy"))
	sharedJSON := parseJSONASTFixture(t, filepath.Join(root, "__shared.json"))

	var count int
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" || filepath.Base(path) == "__shared.json" {
			return nil
		}
		idlPath := strings.TrimSuffix(path, ".json") + ".smithy"
		if _, err := os.Stat(idlPath); err != nil {
			t.Fatalf("JSON fixture %s has no .smithy sibling", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		count++
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			got := assembleJSONFixture(t, sharedJSON, parseJSONASTFixture(t, path))
			want := assembleJSONFixture(t, sharedIDL, parseFile(t, idlPath))

			for _, event := range validation.Validate(got) {
				if event.Severity == validation.Danger {
					t.Errorf("JSON AST produced danger event: %s [%s]", event.String(), event.ID)
				}
			}
			compareModels(t, got, want)
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no JSON AST fixtures compared")
	}
	t.Logf("compared %d JSON AST fixtures", count)
}

// TestSharedJSONMatchesIDLPort asserts that the JSON AST version of the
// upstream __shared fixture and the hand-written IDL port define the same
// model, so the two cannot drift apart.
func TestSharedJSONMatchesIDLPort(t *testing.T) {
	got := assembleJSONFixture(t,
		parseJSONASTFixture(t, filepath.Join(testdataRoot, "loader", "valid", "__shared.json")))
	want := assembleJSONFixture(t,
		parseFile(t, filepath.Join(testdataRoot, "shared.smithy")))
	compareModels(t, got, want)
}

// compareModels compares the semantically meaningful parts of two models.
// Source locations, the syntactic shape-ID marker, `use` statements, elided
// member markers, `for` resource bindings, and the Shape.Doc shortcut are
// intentionally ignored: JSON AST has no equivalent for them, and
// documentation is compared as the trait that both representations share.
func compareModels(t *testing.T, got, want *model.Model) {
	t.Helper()
	compareMetadata(t, got, want)

	wantShapes := want.Shapes()
	gotShapes := got.Shapes()
	if len(gotShapes) != len(wantShapes) {
		t.Errorf("model has %d shapes, want %d", len(gotShapes), len(wantShapes))
	}
	for _, wantShape := range wantShapes {
		gotShape, ok := got.Shape(wantShape.ID)
		if !ok {
			t.Errorf("shape %s missing from JSON AST model", wantShape.ID)
			continue
		}
		compareShape(t, gotShape, wantShape)
	}
	for _, gotShape := range gotShapes {
		if _, ok := want.Shape(gotShape.ID); !ok {
			t.Errorf("JSON AST model has unexpected shape %s", gotShape.ID)
		}
	}
}

func compareMetadata(t *testing.T, got, want *model.Model) {
	t.Helper()
	gotKeys := got.MetadataKeys()
	wantKeys := want.MetadataKeys()
	if len(gotKeys) != len(wantKeys) {
		t.Errorf("metadata keys = %v, want %v", gotKeys, wantKeys)
	}
	for _, key := range wantKeys {
		gotValue, ok := got.Metadata(key)
		if !ok {
			t.Errorf("metadata %q missing", key)
			continue
		}
		wantValue, _ := want.Metadata(key)
		if !nodeEqual(gotValue, wantValue) {
			t.Errorf("metadata %q = %s, want %s", key, gotValue, wantValue)
		}
	}
}

func compareShape(t *testing.T, got, want *model.Shape) {
	t.Helper()
	label := want.ID.String()
	if got.Type != want.Type {
		t.Errorf("%s type = %s, want %s", label, got.Type, want.Type)
	}
	compareModelTraits(t, label, got.Traits, want.Traits)

	if len(got.Members) != len(want.Members) {
		t.Errorf("%s has %d members, want %d", label, len(got.Members), len(want.Members))
	}
	for _, wantMember := range want.Members {
		name := wantMember.ID.Member
		gotMember, ok := got.GetMember(name)
		if !ok {
			t.Errorf("%s member %s missing", label, name)
			continue
		}
		if gotMember.Target != wantMember.Target {
			t.Errorf("%s target = %s, want %s", wantMember.ID, gotMember.Target, wantMember.Target)
		}
		compareModelTraits(t, wantMember.ID.String(), gotMember.Traits, wantMember.Traits)
	}

	if got.Input != want.Input {
		t.Errorf("%s input = %s, want %s", label, got.Input, want.Input)
	}
	if got.Output != want.Output {
		t.Errorf("%s output = %s, want %s", label, got.Output, want.Output)
	}
	compareIDSets(t, label+" errors", got.Errors, want.Errors)
	compareIDSets(t, label+" mixins", got.Mixins, want.Mixins)

	switch want.Type {
	case model.TypeService:
		compareServiceShape(t, got, want)
	case model.TypeResource:
		compareResourceShape(t, got, want)
	}
}

func compareServiceShape(t *testing.T, got, want *model.Shape) {
	t.Helper()
	if got.Service == nil || want.Service == nil {
		t.Errorf("%s service properties missing", want.ID)
		return
	}
	if got.Service.Version != want.Service.Version {
		t.Errorf("%s version = %q, want %q", want.ID, got.Service.Version, want.Service.Version)
	}
	compareIDSets(t, want.ID.String()+" operations", got.Service.Operations, want.Service.Operations)
	compareIDSets(t, want.ID.String()+" resources", got.Service.Resources, want.Service.Resources)
	compareIDSets(t, want.ID.String()+" errors", got.Service.Errors, want.Service.Errors)
	if len(got.Service.Rename) != len(want.Service.Rename) {
		t.Errorf("%s rename = %v, want %v", want.ID, got.Service.Rename, want.Service.Rename)
	}
	for id, name := range want.Service.Rename {
		if got.Service.Rename[id] != name {
			t.Errorf("%s rename %s = %q, want %q", want.ID, id, got.Service.Rename[id], name)
		}
	}
}

func compareResourceShape(t *testing.T, got, want *model.Shape) {
	t.Helper()
	if got.Resource == nil || want.Resource == nil {
		t.Errorf("%s resource properties missing", want.ID)
		return
	}
	for _, entry := range []struct {
		name      string
		got, want model.ShapeID
	}{
		{"create", got.Resource.Create, want.Resource.Create},
		{"read", got.Resource.Read, want.Resource.Read},
		{"update", got.Resource.Update, want.Resource.Update},
		{"delete", got.Resource.Delete, want.Resource.Delete},
		{"list", got.Resource.List, want.Resource.List},
		{"put", got.Resource.Put, want.Resource.Put},
	} {
		if entry.got != entry.want {
			t.Errorf("%s %s = %s, want %s", want.ID, entry.name, entry.got, entry.want)
		}
	}
	compareIDSets(t, want.ID.String()+" operations", got.Resource.Operations, want.Resource.Operations)
	compareIDSets(t, want.ID.String()+" collection operations", got.Resource.CollectionOperations, want.Resource.CollectionOperations)
	compareIDSets(t, want.ID.String()+" resources", got.Resource.Resources, want.Resource.Resources)
	compareRefMaps(t, want.ID.String()+" identifiers", got.Resource.Identifiers, want.Resource.Identifiers)
	compareRefMaps(t, want.ID.String()+" properties", got.Resource.Properties, want.Resource.Properties)
}

func compareModelTraits(t *testing.T, label string, got, want model.TraitMap) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s has %d traits, want %d (got %v, want %v)", label, len(got), len(want), got.SortedIDs(), want.SortedIDs())
	}
	for _, id := range want.SortedIDs() {
		gotTrait, ok := got.Get(id)
		if !ok {
			t.Errorf("%s missing trait %s", label, id)
			continue
		}
		if !nodeEqual(gotTrait.Value, want[id].Value) {
			t.Errorf("%s trait %s = %s, want %s", label, id, gotTrait.Value, want[id].Value)
		}
	}
	for _, id := range got.SortedIDs() {
		if _, ok := want.Get(id); !ok {
			t.Errorf("%s has unexpected trait %s = %s", label, id, got[id].Value)
		}
	}
}

func compareIDSets(t *testing.T, label string, got, want []model.ShapeID) {
	t.Helper()
	gotIDs := make([]string, 0, len(got))
	for _, id := range got {
		gotIDs = append(gotIDs, id.String())
	}
	wantIDs := make([]string, 0, len(want))
	for _, id := range want {
		wantIDs = append(wantIDs, id.String())
	}
	sort.Strings(gotIDs)
	sort.Strings(wantIDs)
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Errorf("%s = %v, want %v", label, gotIDs, wantIDs)
	}
}

func compareRefMaps(t *testing.T, label string, got, want map[string]model.ShapeID) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
	for name, id := range want {
		if got[name] != id {
			t.Errorf("%s[%s] = %s, want %s", label, name, got[name], id)
		}
	}
}

// nodeEqual compares node values while ignoring source locations and the
// syntactic-shape-ID marker (JSON AST has no way to distinguish a string from
// a shape ID). Numbers are compared numerically because the IDL `10e0` and the
// serialized JSON `10.0` are the same value.
func nodeEqual(a, b model.Node) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case model.NodeNull:
		return true
	case model.NodeBoolean:
		return a.Bool == b.Bool
	case model.NodeNumber:
		return numberEqual(a.Num, b.Num)
	case model.NodeString:
		return a.Str == b.Str
	case model.NodeArray:
		if len(a.Arr) != len(b.Arr) {
			return false
		}
		for i := range a.Arr {
			if !nodeEqual(a.Arr[i], b.Arr[i]) {
				return false
			}
		}
		return true
	case model.NodeObject:
		if len(a.Obj) != len(b.Obj) {
			return false
		}
		for _, entry := range a.Obj {
			other, ok := b.Get(entry.Key)
			if !ok || !nodeEqual(entry.Value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func numberEqual(a, b string) bool {
	if a == b {
		return true
	}
	left, ok := new(big.Rat).SetString(a)
	if !ok {
		return false
	}
	right, ok := new(big.Rat).SetString(b)
	if !ok {
		return false
	}
	return left.Cmp(right) == 0
}
