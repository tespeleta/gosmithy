package model_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

const testdataRoot = "../testdata"

func parseFile(t *testing.T, path string) *idl.File {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	f, err := idl.ParseWithOptions(filepath.Base(path), string(src), idl.Options{
		Version:        idl.V2_1,
		DefaultVersion: idl.V1_0,
	})
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return f
}

func assemble(t *testing.T, opts model.Options, paths ...string) *model.Model {
	t.Helper()
	files := make([]*idl.File, 0, len(paths))
	for _, path := range paths {
		files = append(files, parseFile(t, path))
	}
	m, err := model.Assemble(files, opts)
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	return m
}

func dangerEvents(m *model.Model) []validation.Event {
	var out []validation.Event
	for _, e := range validation.Validate(m) {
		if e.Severity == validation.Danger {
			out = append(out, e)
		}
	}
	return out
}

func TestPreludeLoads(t *testing.T) {
	m := assemble(t, model.Options{}, filepath.Join(testdataRoot, "weather.smithy"))
	for _, id := range []string{
		"smithy.api#String",
		"smithy.api#Unit",
		"smithy.api#trait",
		"smithy.api#required",
		"smithy.api#error",
		"smithy.api#mixin",
		"smithy.api#enumValue",
	} {
		shape, ok := m.Shape(model.MustShapeID(id))
		if !ok {
			t.Fatalf("prelude shape %s not found", id)
		}
		if !shape.IsPrelude() {
			t.Fatalf("prelude shape %s is not marked as prelude", id)
		}
	}
	unit, _ := m.Shape(model.MustShapeID("smithy.api#Unit"))
	if !unit.HasTrait(model.TraitUnitType) {
		t.Errorf("smithy.api#Unit should have the unitType trait")
	}
	if events := dangerEvents(m); len(events) > 0 {
		for _, e := range events {
			t.Errorf("unexpected danger event: %s [%s]", e.String(), e.ID)
		}
	}
}

func TestWeatherModel(t *testing.T) {
	m := assemble(t, model.Options{}, filepath.Join(testdataRoot, "weather.smithy"))
	if events := dangerEvents(m); len(events) > 0 {
		for _, e := range events {
			t.Errorf("unexpected danger event: %s [%s]", e.String(), e.ID)
		}
	}

	service, ok := m.Shape(model.MustShapeID("example.weather#Weather"))
	if !ok || service.Type != model.TypeService {
		t.Fatalf("Weather service missing or wrong type")
	}
	if service.Service == nil || service.Service.Version != "2006-03-01" {
		t.Fatalf("Weather version = %#v", service.Service)
	}
	if len(service.Service.Resources) != 1 || service.Service.Resources[0] != model.MustShapeID("example.weather#City") {
		t.Fatalf("Weather resources = %v", service.Service.Resources)
	}

	city, ok := m.Shape(model.MustShapeID("example.weather#City"))
	if !ok || city.Resource == nil {
		t.Fatalf("City resource missing")
	}
	if city.Resource.Read != model.MustShapeID("example.weather#GetCity") {
		t.Errorf("City read = %s", city.Resource.Read)
	}
	if city.Resource.List != model.MustShapeID("example.weather#ListCities") {
		t.Errorf("City list = %s", city.Resource.List)
	}
	if city.Resource.Identifiers["cityId"] != model.MustShapeID("example.weather#CityId") {
		t.Errorf("City identifiers = %v", city.Resource.Identifiers)
	}
	if len(city.Resource.Resources) != 1 || city.Resource.Resources[0] != model.MustShapeID("example.weather#Forecast") {
		t.Errorf("City nested resources = %v", city.Resource.Resources)
	}

	getCity, ok := m.Shape(model.MustShapeID("example.weather#GetCity"))
	if !ok || getCity.Input != model.MustShapeID("example.weather#GetCityInput") {
		t.Errorf("GetCity input = %s", getCity.Input)
	}
	if getCity.Output != model.MustShapeID("example.weather#GetCityOutput") {
		t.Errorf("GetCity output = %s", getCity.Output)
	}
	if len(getCity.Errors) != 1 || getCity.Errors[0] != model.MustShapeID("example.weather#NoSuchResource") {
		t.Errorf("GetCity errors = %v", getCity.Errors)
	}

	// Output members resolve, and member shapes are first-class.
	output, _ := m.Shape(model.MustShapeID("example.weather#GetCityOutput"))
	name, ok := output.GetMember("name")
	if !ok {
		t.Fatalf("GetCityOutput$name missing")
	}
	if name.Target != model.MustShapeID("smithy.api#String") {
		t.Errorf("GetCityOutput$name target = %s", name.Target)
	}
	if !name.Required() {
		t.Errorf("GetCityOutput$name should be required")
	}
	if _, ok := m.Shape(model.MustShapeID("example.weather#GetCityOutput$name")); !ok {
		t.Errorf("member shape GetCityOutput$name is not first-class")
	}
	if name.Source.Line == 0 {
		t.Errorf("member shape has no source location")
	}
	// apply statement becomes a documentation trait.
	input, _ := m.Shape(model.MustShapeID("example.weather#ListCitiesInput"))
	if doc, ok := input.Documentation(); !ok || doc != "hello!" {
		t.Errorf("ListCitiesInput documentation = %q, %v", doc, ok)
	}

	// Operation without an input defaults to smithy.api#Unit.
	currentTime, _ := m.Shape(model.MustShapeID("example.weather#GetCurrentTime"))
	if currentTime.Input != model.TraitUnit {
		t.Errorf("GetCurrentTime input = %s, want Unit", currentTime.Input)
	}
	if currentTime.Output != model.MustShapeID("example.weather#GetCurrentTimeOutput") {
		t.Errorf("GetCurrentTime output = %s", currentTime.Output)
	}

	closure, ok := m.ServiceClosure(model.MustShapeID("example.weather#Weather"))
	if !ok {
		t.Fatalf("service closure not available")
	}
	inClosure := make(map[model.ShapeID]bool, len(closure))
	for _, id := range closure {
		inClosure[id] = true
	}
	for _, want := range []string{
		"example.weather#Weather",
		"example.weather#City",
		"example.weather#Forecast",
		"example.weather#GetCity",
		"example.weather#GetCityInput",
		"example.weather#GetCityInput$cityId",
		"example.weather#NoSuchResource",
		"example.weather#CitySummaries",
		"example.weather#CitySummary",
		"smithy.api#Unit",
	} {
		if !inClosure[model.MustShapeID(want)] {
			t.Errorf("expected %s in service closure", want)
		}
	}
	if !inClosure[model.MustShapeID("smithy.api#String")] {
		t.Errorf("smithy.api#String should be in the service closure via member targets")
	}
}

func TestWeatherJSONIsDeterministic(t *testing.T) {
	m := assemble(t, model.Options{}, filepath.Join(testdataRoot, "weather.smithy"))
	first, err := m.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		next, err := m.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(next) != string(first) {
			t.Fatalf("model JSON is not deterministic")
		}
	}
	if len(first) < 100 {
		t.Fatalf("model JSON is suspiciously small: %d bytes", len(first))
	}
}

func TestValidFixturesAssemble(t *testing.T) {
	root := filepath.Join(testdataRoot, "loader", "valid")
	shared := parseFile(t, filepath.Join(testdataRoot, "shared.smithy"))
	var count, skipped int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".smithy" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file := parseFile(t, path)
		if file.Version.Major == 2 && file.Version.Minor >= 1 {
			skipped++
			return nil
		}
		if filepath.Base(path) == "__shared.smithy" {
			return nil
		}
		count++
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			m, err := model.Assemble([]*idl.File{shared, file}, model.Options{})
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range validation.Validate(m) {
				if e.Severity == validation.Danger {
					t.Errorf("%s [%s]", e.String(), e.ID)
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("assembled %d valid fixtures (skipped %d 2.1 fixtures)", count, skipped)
}
