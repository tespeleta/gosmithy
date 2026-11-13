package model_test

import (
	"path/filepath"
	"testing"

	"github.com/tespeleta/gosmithy/model"
)

// TestMixinFlatteningMatchesFixtures compares the semantic model produced by
// flattening mixins with the upstream ".flattened.smithy" fixture. The
// flattened fixture is assembled as a normal model; only concrete shapes are
// compared because mixins are implementation details.
func TestMixinFlatteningMatchesFixtures(t *testing.T) {
	cases := []string{
		"loads-mixins",
		"mixins-with-members",
		"mixins-with-members-and-traits",
		"mixins-with-conflicting-trait-values",
		"mixins-with-member-override-1",
		"mixins-with-member-override-2",
		"mixins-with-member-override-3",
		"mixins-with-member-override-4",
		"enum-mixins",
		"idl-mixins-redefine-member",
		"services",
		"resources",
		"operations",
		"mixins-with-mixin-local-traits",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			got := assemble(t, model.Options{}, filepath.Join(testdataRoot, "loader", "valid", "mixins", name+".smithy"))
			want := assemble(t, model.Options{}, filepath.Join(testdataRoot, "loader", "valid", "mixins", name+".flattened.smithy"))
			compareConcreteShapes(t, got, want)
		})
	}
}

func compareConcreteShapes(t *testing.T, got, want *model.Model) {
	t.Helper()
	compared := 0
	for _, wantShape := range want.Shapes() {
		if wantShape.IsPrelude() || wantShape.IsMember() || wantShape.IsMixin() {
			continue
		}
		gotShape, ok := got.Shape(wantShape.ID)
		if !ok {
			t.Errorf("shape %s missing from assembled model", wantShape.ID)
			continue
		}
		compared++
		if gotShape.Type != wantShape.Type {
			t.Errorf("%s type = %s, want %s", wantShape.ID, gotShape.Type, wantShape.Type)
			continue
		}
		compareTraits(t, wantShape.ID, gotShape.Traits, wantShape.Traits)
		for _, wantMember := range wantShape.Members {
			gotMember, ok := gotShape.GetMember(wantMember.ID.Member)
			if !ok {
				t.Errorf("%s member %s missing", wantShape.ID, wantMember.ID.Member)
				continue
			}
			if gotMember.Target != wantMember.Target {
				t.Errorf("%s target = %s, want %s", wantMember.ID, gotMember.Target, wantMember.Target)
			}
			compareTraits(t, wantMember.ID, gotMember.Traits, wantMember.Traits)
		}
		if len(gotShape.Members) != len(wantShape.Members) {
			t.Errorf("%s has %d members, want %d", wantShape.ID, len(gotShape.Members), len(wantShape.Members))
		}
		switch wantShape.Type {
		case model.TypeOperation:
			if gotShape.Input != wantShape.Input {
				t.Errorf("%s input = %s, want %s", wantShape.ID, gotShape.Input, wantShape.Input)
			}
			if gotShape.Output != wantShape.Output {
				t.Errorf("%s output = %s, want %s", wantShape.ID, gotShape.Output, wantShape.Output)
			}
			if len(gotShape.Errors) != len(wantShape.Errors) {
				t.Errorf("%s errors = %v, want %v", wantShape.ID, gotShape.Errors, wantShape.Errors)
			}
		case model.TypeService:
			compareServices(t, gotShape, wantShape)
		case model.TypeResource:
			compareResources(t, gotShape, wantShape)
		}
	}
	if compared == 0 {
		t.Fatal("no concrete shapes compared")
	}
}

func compareTraits(t *testing.T, id model.ShapeID, got, want model.TraitMap) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s has %d traits, want %d (got %v, want %v)", id, len(got), len(want), got.SortedIDs(), want.SortedIDs())
	}
	for _, traitID := range want.SortedIDs() {
		gotTrait, ok := got.Get(traitID)
		if !ok {
			t.Errorf("%s missing trait %s", id, traitID)
			continue
		}
		if !gotTrait.Value.Equal(want[traitID].Value) {
			t.Errorf("%s trait %s = %s, want %s", id, traitID, gotTrait.Value, want[traitID].Value)
		}
	}
	for _, traitID := range got.SortedIDs() {
		if _, ok := want.Get(traitID); !ok {
			t.Errorf("%s has unexpected trait %s = %s", id, traitID, got[traitID].Value)
		}
	}
}

func compareServices(t *testing.T, got, want *model.Shape) {
	t.Helper()
	if got.Service == nil || want.Service == nil {
		t.Errorf("%s service properties missing", want.ID)
		return
	}
	if got.Service.Version != want.Service.Version {
		t.Errorf("%s version = %q, want %q", want.ID, got.Service.Version, want.Service.Version)
	}
	compareIDLists(t, want.ID.String()+" operations", got.Service.Operations, want.Service.Operations)
	compareIDLists(t, want.ID.String()+" resources", got.Service.Resources, want.Service.Resources)
	compareIDLists(t, want.ID.String()+" errors", got.Service.Errors, want.Service.Errors)
}

func compareResources(t *testing.T, got, want *model.Shape) {
	t.Helper()
	if got.Resource == nil || want.Resource == nil {
		t.Errorf("%s resource properties missing", want.ID)
		return
	}
	compareIDLists(t, want.ID.String()+" operations", got.Resource.Operations, want.Resource.Operations)
	compareIDLists(t, want.ID.String()+" resources", got.Resource.Resources, want.Resource.Resources)
	compareIDLists(t, want.ID.String()+" collectionOperations", got.Resource.CollectionOperations, want.Resource.CollectionOperations)
	for name, id := range want.Resource.Identifiers {
		if got.Resource.Identifiers[name] != id {
			t.Errorf("%s identifier %s = %s, want %s", want.ID, name, got.Resource.Identifiers[name], id)
		}
	}
	for name, id := range want.Resource.Properties {
		if got.Resource.Properties[name] != id {
			t.Errorf("%s property %s = %s, want %s", want.ID, name, got.Resource.Properties[name], id)
		}
	}
}

func compareIDLists(t *testing.T, label string, got, want []model.ShapeID) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", label, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %s, want %s", label, i, got[i], want[i])
		}
	}
}
