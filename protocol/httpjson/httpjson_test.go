package httpjson

import (
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
)

func TestDefinitionAssembles(t *testing.T) {
	file, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.Assemble([]*idl.File{file}, model.Options{})
	if err != nil {
		t.Fatal(err)
	}
	shape, ok := m.Shape(ID)
	if !ok {
		t.Fatalf("protocol trait %s is not in the model", ID)
	}
	if shape.Type != model.TypeStructure {
		t.Errorf("%s is a %s shape, want structure", ID, shape.Type)
	}
	if !shape.HasTrait(model.TraitTrait) {
		t.Errorf("%s is not marked @trait", ID)
	}
	if !shape.HasTrait(model.TraitProtocolDef) {
		t.Errorf("%s is not marked @protocolDefinition", ID)
	}
	trait, ok := shape.GetTrait(model.TraitProtocolDef)
	if !ok {
		t.Fatal("missing protocolDefinition trait")
	}
	traits, ok := trait.Value.Get("traits")
	if !ok || traits.Kind != model.NodeArray {
		t.Fatalf("protocolDefinition.traits = %v", trait.Value)
	}
	found := map[string]bool{}
	for _, entry := range traits.Arr {
		value, ok := entry.AsString()
		if !ok {
			continue
		}
		found[value] = true
	}
	for _, want := range []string{
		model.TraitHTTP.String(),
		model.TraitHTTPError.String(),
		model.TraitHTTPHeader.String(),
		model.TraitHTTPLabel.String(),
		model.TraitHTTPPayload.String(),
		model.TraitHTTPPrefix.String(),
		model.TraitHTTPQuery.String(),
		model.TraitHTTPQueryParams.String(),
		model.TraitHTTPResponse.String(),
		model.TraitJSONName.String(),
		model.TraitMediaType.String(),
		model.TraitTimestampFmt.String(),
	} {
		if !found[want] {
			t.Errorf("protocol definition does not list %s", want)
		}
	}
}

func TestSelect(t *testing.T) {
	for _, name := range []string{TraitID, "httpJson"} {
		protocol, file, err := Select(name)
		if err != nil {
			t.Fatalf("Select(%q): %v", name, err)
		}
		if protocol.ID != ID {
			t.Errorf("Select(%q).ID = %s, want %s", name, protocol.ID, ID)
		}
		if file == nil {
			t.Errorf("Select(%q) returned a nil definition", name)
		}
	}
	if _, _, err := Select("awsJson1_1"); err == nil {
		t.Fatal("Select accepted an unknown protocol")
	}
}
