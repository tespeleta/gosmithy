package httpbinding

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
)

func TestIndexHTTPFixture(t *testing.T) {
	m, err := assemble(`$version: "2.0"
namespace example

service Example {
    operations: [GetThing, SearchThings, UploadData, DownloadData, HeaderEcho, SetStatus]
}

@error("client")
@httpError(404)
structure NotFound {
    @required
    message: String
}

structure GetThingInput {
    @required @httpLabel id: String
    @httpHeader("X-Trace") trace: String
}
structure GetThingOutput {
    @required thing: String
    @httpResponseCode status: Integer
}
@http(method: "GET", uri: "/things/{id}")
operation GetThing {
    input: GetThingInput
    output: GetThingOutput
    errors: [NotFound]
}

structure SearchInput {
    @httpQuery("q") query: String
    @httpQuery("tag") tags: TagList
    @httpQueryParams params: StringMap
}
list TagList { member: String }
map StringMap { key: String, value: String }
@http(method: "GET", uri: "/things")
operation SearchThings {
    input: SearchInput
}

structure UploadInput {
    @required @httpLabel id: String
    @httpPayload data: Blob
}
@http(method: "PUT", uri: "/things/{id}/data")
operation UploadData {
    input: UploadInput
}

structure DownloadInput {
    @required @httpLabel id: String
}
structure DownloadOutput {
    @httpPayload data: Blob
}
@http(method: "GET", uri: "/things/{id}/data")
operation DownloadData {
    input: DownloadInput
    output: DownloadOutput
}

structure HeaderInput {
    @httpHeader("X-String") headerString: String
    @httpHeader("X-List") headerList: TagList
    @httpPrefixHeaders("X-Meta-") extra: StringMap
}
@http(method: "POST", uri: "/headers")
operation HeaderEcho {
    input: HeaderInput
}

structure StatusOutput {
    @required @httpResponseCode status: Integer
    @required message: String
}
@http(method: "POST", uri: "/status", code: 201)
operation SetStatus {
    output: StatusOutput
}
`)
	if err != nil {
		t.Fatal(err)
	}
	bindings, diagnostics := Index(m, model.MustShapeID("example#Example"))
	for _, d := range diagnostics {
		t.Errorf("unexpected diagnostic: %s", d)
	}
	if len(bindings) != 6 {
		t.Fatalf("bindings = %d, want 6", len(bindings))
	}
	byName := make(map[string]*Binding)
	for _, binding := range bindings {
		byName[binding.Operation.Name] = binding
	}

	get := byName["GetThing"]
	if get.Method != "GET" || get.URI != "/things/{id}" {
		t.Errorf("GetThing = %s %s", get.Method, get.URI)
	}
	if len(get.Segments) != 2 || !get.Segments[1].IsLabel || get.Segments[1].Label != "id" {
		t.Errorf("GetThing segments = %+v", get.Segments)
	}
	if _, ok := get.Input.Labels["id"]; !ok {
		t.Errorf("GetThing labels = %+v", get.Input.Labels)
	}
	if _, ok := get.Input.Headers["X-Trace"]; !ok {
		t.Errorf("GetThing headers = %+v", get.Input.Headers)
	}
	if get.SuccessCode != 200 {
		t.Errorf("GetThing success = %d", get.SuccessCode)
	}
	if len(get.Output.Body) != 1 {
		t.Errorf("GetThing output body = %+v", get.Output.Body)
	}
	if get.Output.ResponseCode.IsZero() {
		t.Errorf("GetThing response code member is not set")
	}
	if len(get.Errors) != 1 || get.Errors[0].Code != 404 {
		t.Errorf("GetThing errors = %+v", get.Errors)
	}

	search := byName["SearchThings"]
	if search.Input.Query["q"].Member != "query" {
		t.Errorf("SearchThings query = %+v", search.Input.Query)
	}
	if len(search.Input.Query) != 2 {
		t.Errorf("SearchThings query count = %d", len(search.Input.Query))
	}
	if search.Input.QueryParams.IsZero() {
		t.Errorf("SearchThings query params not set")
	}

	upload := byName["UploadData"]
	if upload.Input.Payload.IsZero() {
		t.Errorf("UploadData payload not set")
	}
	if len(upload.Input.Body) != 0 {
		t.Errorf("UploadData body = %+v", upload.Input.Body)
	}

	header := byName["HeaderEcho"]
	if header.Input.PrefixHeader != "X-Meta-" || header.Input.PrefixHeaders.IsZero() {
		t.Errorf("HeaderEcho prefix headers = %+v", header.Input)
	}

	status := byName["SetStatus"]
	if status.SuccessCode != 201 {
		t.Errorf("SetStatus success = %d", status.SuccessCode)
	}
	if status.Output.ResponseCode.IsZero() {
		t.Errorf("SetStatus response code member not set")
	}
}

func TestMissingHTTP(t *testing.T) {
	m := mustAssemble(t, `$version: "2.0"
namespace example
service Example { operations: [Op] }
operation Op {}
`)
	_, diagnostics := Index(m, model.MustShapeID("example#Example"))
	requireEvent(t, diagnostics, EventMissingHTTP)
}

func TestLabelMustBeRequired(t *testing.T) {
	m := mustAssemble(t, `$version: "2.0"
namespace example
service Example { operations: [Op] }
structure In {
    @httpLabel id: String
}
@http(method: "GET", uri: "/things/{id}")
operation Op { input: In }
`)
	_, diagnostics := Index(m, model.MustShapeID("example#Example"))
	requireEvent(t, diagnostics, EventInvalidLabel)
}

func TestLabelMustMatchURI(t *testing.T) {
	m := mustAssemble(t, `$version: "2.0"
namespace example
service Example { operations: [Op] }
structure In {
    @required @httpLabel id: String
}
@http(method: "GET", uri: "/things/{other}")
operation Op { input: In }
`)
	_, diagnostics := Index(m, model.MustShapeID("example#Example"))
	requireEvent(t, diagnostics, EventInvalidLabel)
}

func TestDuplicateBinding(t *testing.T) {
	m := mustAssemble(t, `$version: "2.0"
namespace example
service Example { operations: [Op] }
structure In {
    @required
    @httpLabel
    @httpHeader("X-Id")
    id: String
}
@http(method: "GET", uri: "/things/{id}")
operation Op { input: In }
`)
	_, diagnostics := Index(m, model.MustShapeID("example#Example"))
	requireEvent(t, diagnostics, EventDuplicateBinding)
}

func TestRouteOverlap(t *testing.T) {
	m := mustAssemble(t, `$version: "2.0"
namespace example
service Example { operations: [First, Second, Third] }
@http(method: "GET", uri: "/things/{id}")
operation First {}
@http(method: "GET", uri: "/things/special")
operation Second {}
@http(method: "POST", uri: "/things/special")
operation Third {}
`)
	_, diagnostics := Index(m, model.MustShapeID("example#Example"))
	requireEvent(t, diagnostics, EventRouteConflict)
	// A route that only differs by method is not an overlap.
	for _, d := range diagnostics {
		if d.ID == EventRouteConflict && strings.Contains(d.Message, "Third") {
			t.Errorf("different-method route reported as overlap: %s", d)
		}
	}
}

func TestPayloadAndBodyConflict(t *testing.T) {
	m := mustAssemble(t, `$version: "2.0"
namespace example
service Example { operations: [Op] }
structure In {
    @httpPayload data: Blob
    other: String
}
@http(method: "POST", uri: "/upload")
operation Op { input: In }
`)
	_, diagnostics := Index(m, model.MustShapeID("example#Example"))
	requireEvent(t, diagnostics, EventInvalidPayload)
}

func TestInvalidQueryTarget(t *testing.T) {
	m := mustAssemble(t, `$version: "2.0"
namespace example
service Example { operations: [Op] }
structure In {
    @httpQuery("data") data: Blob
}
@http(method: "GET", uri: "/search")
operation Op { input: In }
`)
	_, diagnostics := Index(m, model.MustShapeID("example#Example"))
	requireEvent(t, diagnostics, EventInvalidTarget)
}

func TestGreedyLabelIsUnsupported(t *testing.T) {
	m := mustAssemble(t, `$version: "2.0"
namespace example
service Example { operations: [Op] }
structure In {
    @required @httpLabel path: String
}
@http(method: "GET", uri: "/files/{path+}")
operation Op { input: In }
`)
	_, diagnostics := Index(m, model.MustShapeID("example#Example"))
	requireEvent(t, diagnostics, EventInvalidURI)
}

// --- helpers ------------------------------------------------------------------

func mustAssemble(t *testing.T, src string) *model.Model {
	t.Helper()
	m, err := assemble(src)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func assemble(src string) (*model.Model, error) {
	file, err := idl.ParseWithOptions("test.smithy", src, idl.Options{Version: idl.V2_1, DefaultVersion: idl.V2_0})
	if err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}
	return model.Assemble([]*idl.File{file}, model.Options{})
}

func requireEvent(t *testing.T, diagnostics []model.Diagnostic, eventID string) {
	t.Helper()
	for _, d := range diagnostics {
		if d.ID == eventID {
			return
		}
	}
	t.Fatalf("diagnostics = %v, want event %s", diagnostics, eventID)
}
