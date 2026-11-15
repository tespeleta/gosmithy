package openapi

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
)

var updateGolden = flag.Bool("update-openapi", false, "update OpenAPI golden files")

func assembleFixture(t *testing.T, path string) *model.Model {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := idl.ParseWithOptions(path, string(data), idl.Options{Version: idl.V2_1, DefaultVersion: idl.V2_0})
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.Assemble([]*idl.File{file}, model.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func requestFor(m *model.Model, service string) *codegen.Request {
	return &codegen.Request{
		Model:    m,
		Service:  model.MustShapeID(service),
		Protocol: &codegen.Protocol{ID: httpjson.ID},
	}
}

func TestGoldenLibrary(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "library.smithy"))
	doc, diagnostics := Build(requestFor(m, "example.library#Library"))
	for _, d := range diagnostics {
		if d.Severity == model.SeverityDanger {
			t.Fatalf("unexpected danger: %s", d.String())
		}
	}
	data, err := doc.JSON()
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "golden", "library.openapi.json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, want) {
		t.Errorf("OpenAPI output differs from %s; run go test ./codegen/openapi -update-openapi", golden)
	}
}

func TestBuildLibrary(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "library.smithy"))
	doc, diagnostics := Build(requestFor(m, "example.library#Library"))
	if doc == nil {
		t.Fatalf("document is nil; diagnostics: %v", diagnostics)
	}
	if doc.Info.Title != "Library" || doc.Info.Version != "1.0.0" {
		t.Errorf("info = %+v", doc.Info)
	}
	if len(doc.Paths) != 3 {
		t.Errorf("paths = %v", len(doc.Paths))
	}
	search := doc.Paths["/books/search"].Post
	if search == nil {
		t.Fatal("missing Search operation")
	}
	if search.RequestBody == nil || search.RequestBody.Content["application/json"].Schema == nil {
		t.Fatalf("missing request body: %+v", search.RequestBody)
	}
	if search.Responses["400"] == nil {
		t.Errorf("missing error response: %+v", search.Responses)
	}
	list := doc.Paths["/books"].Get
	if list == nil || len(list.Parameters) != 1 {
		t.Fatalf("list parameters = %+v", list)
	}
	schema := list.Parameters[0].Schema
	if schema == nil || schema.MinItems == nil || *schema.MinItems != 1 || schema.MaxItems == nil || *schema.MaxItems != 20 {
		t.Errorf("query constraints = %+v", schema)
	}
	if doc.Components == nil || doc.Components.Schemas["Book"] == nil ||
		doc.Components.Schemas["SearchResult"] == nil {
		t.Errorf("components = %+v", doc.Components)
	}
}

func TestBuildHTTP(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "http.smithy"))
	doc, diagnostics := Build(requestFor(m, "example.http#HttpJson"))
	if doc == nil {
		t.Fatalf("document is nil; diagnostics: %v", diagnostics)
	}
	warnings := 0
	for _, d := range diagnostics {
		if d.Severity == model.SeverityDanger {
			t.Fatalf("unexpected danger: %s", d.String())
		}
		if d.Severity == model.SeverityWarning {
			warnings++
		}
	}
	if warnings != 5 {
		t.Errorf("warnings = %d, want 5: %v", warnings, diagnostics)
	}
	getThing := doc.Paths["/things/{id}"].Get
	if getThing == nil {
		t.Fatal("missing GetThing")
	}
	if getThing.Responses["200"].Headers["X-Version"] == nil {
		t.Errorf("missing response headers: %+v", getThing.Responses["200"].Headers)
	}
	if _, ok := getThing.Responses["200"].Headers["X-Meta-"]; ok {
		t.Errorf("prefix headers must not be emitted: %+v", getThing.Responses["200"].Headers)
	}
	upload := doc.Paths["/things/{id}/data"].Put
	if upload == nil || upload.RequestBody == nil {
		t.Fatal("missing UploadData request body")
	}
	if upload.RequestBody.Content["application/x-custom"] == nil {
		t.Errorf("payload content type = %+v", upload.RequestBody.Content)
	}
	search := doc.Paths["/things"].Get
	if search == nil {
		t.Fatal("missing SearchThings")
	}
	if len(search.Parameters) != 4 {
		t.Errorf("search parameters = %+v", search.Parameters)
	}
	var since *Parameter
	for _, param := range search.Parameters {
		if param.Name == "since" {
			since = param
		}
	}
	if since == nil || since.Schema.Type != "number" || since.Schema.Format != "double" {
		t.Errorf("timestamp query schema = %+v", since)
	}
	setStatus := doc.Paths["/status"].Post
	if setStatus == nil || setStatus.Responses["201"] == nil {
		t.Fatalf("missing SetStatus response: %+v", setStatus)
	}
	body := setStatus.Responses["201"].Content["application/json"]
	if body == nil || body.Schema == nil {
		t.Fatalf("missing SetStatus body: %+v", setStatus.Responses["201"])
	}
	if _, ok := body.Schema.Properties["status"]; ok {
		t.Errorf("@httpResponseCode member must not be a body property: %+v", body.Schema.Properties)
	}
	if _, ok := body.Schema.Properties["message"]; !ok {
		t.Errorf("missing message body property: %+v", body.Schema.Properties)
	}
}

func TestBuildDocsTraits(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "docs.smithy"))
	doc, diagnostics := Build(requestFor(m, "example.docs#Docs"))
	if doc == nil {
		t.Fatalf("document is nil; diagnostics: %v", diagnostics)
	}
	for _, d := range diagnostics {
		if d.Severity == model.SeverityDanger {
			t.Fatalf("unexpected danger: %s", d.String())
		}
	}
	if doc.Info.Title != "Docs Showcase" || doc.Info.Version != "2.1.0" {
		t.Errorf("info = %+v", doc.Info)
	}
	if got := doc.UnstableFeatures; len(got) != 1 || got[0] != "preview" {
		t.Errorf("unstable features = %v", got)
	}
	if doc.ExternalDocs == nil || doc.ExternalDocs.URL != "https://example.com/" {
		t.Errorf("external docs = %+v", doc.ExternalDocs)
	}
	getBook := doc.Paths["/books/{id}"].Get
	if getBook == nil {
		t.Fatal("missing GetBook operation")
	}
	if len(getBook.Parameters) != 1 || getBook.Parameters[0].Name != "id" {
		t.Fatalf("parameters = %+v", getBook.Parameters)
	}
	if getBook.Parameters[0].Description != "Identifies a book." {
		t.Errorf("parameter description = %q", getBook.Parameters[0].Description)
	}
	if getBook.Parameters[0].Examples == nil {
		t.Errorf("parameter examples missing: %+v", getBook.Parameters[0])
	}
	if len(getBook.Responses["200"].Content["application/json"].Examples) == 0 {
		t.Errorf("response examples missing: %+v", getBook.Responses["200"])
	}
	if len(getBook.Responses["404"].Content["application/json"].Examples) == 0 {
		t.Errorf("error examples missing: %+v", getBook.Responses["404"])
	}
	create := doc.Paths["/books"].Post
	if create == nil || create.RequestBody == nil {
		t.Fatal("missing CreateBook request body")
	}
	if len(create.RequestBody.Content["application/json"].Examples) == 0 {
		t.Errorf("request examples missing: %+v", create.RequestBody.Content["application/json"])
	}
	book := doc.Components.Schemas["Book"]
	if book == nil {
		t.Fatal("missing Book schema")
	}
	if !book.Properties["secret"].Internal || !book.Properties["secret"].Deprecated {
		t.Errorf("secret schema = %+v", book.Properties["secret"])
	}
	if !book.Properties["token"].Recommended || book.Properties["token"].Since != "2.1.0" ||
		book.Properties["token"].Format != "password" {
		t.Errorf("token schema = %+v", book.Properties["token"])
	}
	if !book.Properties["preview"].Unstable {
		t.Errorf("preview schema = %+v", book.Properties["preview"])
	}
	preview := doc.Paths["/books/{id}/preview"].Post
	if preview == nil || !preview.Deprecated || !preview.Unstable {
		t.Errorf("preview operation = %+v", preview)
	}
	foundSynthetic := false
	for _, schema := range doc.Components.Schemas {
		if schema.Synthetic {
			foundSynthetic = true
		}
	}
	if !foundSynthetic {
		t.Errorf("no synthetic schema in components: %+v", doc.Components.Schemas)
	}
	genre := doc.Components.Schemas["Genre"]
	if genre == nil || genre.EnumDescriptions["fiction"] == "" ||
		genre.EnumSince["reference"] != "2.0.0" || !contains(genre.EnumDeprecated, "legacy") {
		t.Errorf("enum metadata = %+v", genre)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestBuildRequiresProtocol(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "library.smithy"))
	doc, diagnostics := Build(&codegen.Request{Model: m, Service: model.MustShapeID("example.library#Library")})
	if doc != nil {
		t.Fatal("expected no document without a protocol")
	}
	if len(diagnostics) == 0 || diagnostics[0].Severity != model.SeverityDanger {
		t.Fatalf("diagnostics = %v", diagnostics)
	}
}

func TestIndexGenerator(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "docs.smithy"))
	req := requestFor(m, "example.docs#Docs")
	response, err := NewIndex().Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte)
	for _, file := range response.Files {
		files[file.Path] = file.Content
	}
	if len(files["docs/index.html"]) == 0 || len(files["docs/openapi.json"]) == 0 {
		t.Fatalf("files = %+v", files)
	}
	html := string(files["docs/index.html"])
	for _, want := range []string{
		`<redoc spec-url="./openapi.json"></redoc>`,
		`<script type="module" src="https://cdn.redoc.ly/redoc/v3.x/bundles/redoc.standalone.js"></script>`,
		"Docs Showcase API Reference",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing %q", want)
		}
	}
	doc, _ := Build(req)
	want, err := doc.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(files["docs/openapi.json"], want) {
		t.Error("docs/openapi.json differs from the OpenAPI builder")
	}
}

func TestDescriptorGeneratorWritesFile(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "library.smithy"))
	response, err := New().Generate(context.Background(), requestFor(m, "example.library#Library"))
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Files) != 1 || response.Files[0].Path != "openapi.json" {
		t.Fatalf("files = %+v", response.Files)
	}
	if !bytes.HasPrefix(response.Files[0].Content, []byte("{\n  \"openapi\": \"3.0.3\"")) {
		t.Errorf("unexpected output: %s", response.Files[0].Content[:60])
	}
}

func TestDeterministic(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "docs.smithy"))
	req := requestFor(m, "example.docs#Docs")
	first, _ := Build(req)
	second, _ := Build(req)
	a, _ := first.JSON()
	b, _ := second.JSON()
	if !bytes.Equal(a, b) {
		t.Error("OpenAPI output is not deterministic")
	}
}

func TestBuildMissingService(t *testing.T) {
	m := assembleFixture(t, filepath.Join("..", "..", "testdata", "library.smithy"))
	req := requestFor(m, "example.library#Missing")
	doc, diagnostics := Build(req)
	if doc != nil || len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "not found") {
		t.Fatalf("doc=%v diagnostics=%v", doc, diagnostics)
	}
}
