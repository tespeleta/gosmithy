package golang

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
	"github.com/tespeleta/gosmithy/validation"
)

func TestProtocolGolden(t *testing.T) {
	t.Parallel()
	m := loadProtocolFixture(t, "../../testdata/http.smithy")
	requireValid(t, m)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("example.http#HttpJson"), "example.com/http")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, d := range diags {
		if d.Severity == validation.Danger {
			t.Errorf("unexpected DANGER diagnostic: %s", d)
		}
	}
	goldenDir := filepath.Join("testdata", "golden", "http")
	if *updateGolden {
		if err := os.RemoveAll(goldenDir); err != nil {
			t.Fatal(err)
		}
		writeFiles(t, goldenDir, files)
	}
	compareGolden(t, goldenDir, files)

	dir := t.TempDir()
	writeFiles(t, dir, files)
	runGo(t, dir, "build", "./...")
	runGo(t, dir, "vet", "./...")
}

func TestProtocolRejectsUnknownProtocol(t *testing.T) {
	m := loadProtocolFixture(t, "../../testdata/http.smithy")
	req := &codegen.Request{
		Model:    m,
		Service:  model.MustShapeID("example.http#HttpJson"),
		Version:  model.V2_0,
		Protocol: &codegen.Protocol{ID: model.MustShapeID("example#other")},
		Target:   codegen.Target{Name: GeneratorName, Generators: []string{GeneratorName}},
		Options:  map[string]string{"module": "example.com/http"},
	}
	resp, err := New().Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := false
	for _, d := range resp.Diagnostics {
		if d.Severity == validation.Danger && strings.Contains(d.Message, "protocol") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %v, want unsupported protocol", resp.Diagnostics)
	}
}

func TestProtocolRouteConflictIsDanger(t *testing.T) {
	m := assembleProtocolSources(t, `$version: "2.0"
namespace example.conflict

use gosmithy.protocols#httpJson

@httpJson
service Conflict {
    operations: [First, Second]
}

@http(method: "GET", uri: "/things/{id}")
operation First {}

@http(method: "GET", uri: "/things/special")
operation Second {}
`)
	_, diags, err := runProtocolGenerator(t, m, model.MustShapeID("example.conflict#Conflict"), "example.com/conflict")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := false
	for _, d := range diags {
		if d.Severity == validation.Danger && strings.Contains(d.Message, "overlaps") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %v, want route overlap", diags)
	}
}

func TestGoPackageName(t *testing.T) {
	cases := map[string]string{
		"example.com/library":     "library",
		"example.com/library-gen": "library_gen",
		"example.com/foo/v2":      "foo",
		"gopkg.in/yaml.v3":        "yaml",
		"example.com/type":        "type_",
		"example.com/9lives":      "pkg_9lives",
		"example.com":             "com",
	}
	for module, want := range cases {
		if got := goPackageName(module); got != want {
			t.Errorf("goPackageName(%q) = %q, want %q", module, got, want)
		}
	}
}

func loadProtocolFixture(t *testing.T, path string) *model.Model {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := idl.ParseWithOptions(path, string(src), idl.Options{Version: idl.V2_1, DefaultVersion: idl.V2_0})
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	definition, err := httpjson.Definition()
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.Assemble([]*idl.File{definition, file}, model.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func assembleProtocolSources(t *testing.T, sources ...string) *model.Model {
	t.Helper()
	files := make([]*idl.File, 0, len(sources))
	for i, src := range sources {
		name := fmt.Sprintf("protocol-test%d.smithy", i)
		file, err := idl.ParseWithOptions(name, src, idl.Options{Version: idl.V2_1, DefaultVersion: idl.V2_0})
		if err != nil {
			t.Fatalf("parsing source %d: %v", i, err)
		}
		files = append(files, file)
	}
	definition, err := httpjson.Definition()
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, definition)
	m, err := model.Assemble(files, model.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func runProtocolGenerator(t *testing.T, m *model.Model, service model.ShapeID, module string) ([]codegen.File, []codegen.Diagnostic, error) {
	t.Helper()
	req := &codegen.Request{
		Model:    m,
		Service:  service,
		Version:  model.V2_0,
		Protocol: &codegen.Protocol{ID: httpjson.ID},
		Target:   codegen.Target{Name: GeneratorName, Generators: []string{GeneratorName}},
		Options:  map[string]string{"module": module, "useOptionals": "false"},
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

func TestGeneratedProtocolRoundTrip(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	m := loadProtocolFixture(t, "../../testdata/http.smithy")
	requireValid(t, m)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("example.http#HttpJson"), "example.com/http")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, d := range diags {
		if d.Severity == validation.Danger {
			t.Fatalf("unexpected DANGER diagnostic: %s", d)
		}
	}
	dir := t.TempDir()
	writeFiles(t, dir, files)
	testFile := filepath.Join(dir, "protocol_e2e_test.go")
	if err := os.WriteFile(testFile, []byte(generatedProtocolTests), 0o644); err != nil {
		t.Fatal(err)
	}
	runGo(t, dir, "test", "./...")
}

const generatedProtocolTests = `package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/http/internal/smithy"
	"example.com/http/types"
)

func ptr[T any](v T) *T { return &v }

var (
	protocolTime       = time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	protocolSeconds    = time.Date(2024, 2, 3, 4, 5, 6, 123456789, time.UTC)
	protocolMillis     = time.Date(2024, 2, 3, 4, 5, 6, 123000000, time.UTC)
	protocolHTTPDate   = time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	protocolDateTime   = time.Date(2024, 2, 3, 4, 5, 6, 987654321, time.UTC)
)

type recordedRequest struct {
	method   string
	path     string
	rawQuery string
	header   http.Header
	body     []byte
}

type recordingTransport struct {
	base     http.RoundTripper
	mu       sync.Mutex
	requests []recordedRequest
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	r.mu.Lock()
	r.requests = append(r.requests, recordedRequest{
		method:   req.Method,
		path:     req.URL.Path,
		rawQuery: req.URL.RawQuery,
		header:   req.Header.Clone(),
		body:     body,
	})
	r.mu.Unlock()
	return r.base.RoundTrip(req)
}

func (r *recordingTransport) last(t *testing.T) recordedRequest {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		t.Fatal("no request was recorded")
	}
	return r.requests[len(r.requests)-1]
}

type testService struct {
	mu           sync.Mutex
	lastSearch   *types.SearchThingsInput
	lastCreate   *types.CreateThingInput
	lastUpload   *types.UploadDataInput
	lastHeader   *types.HeaderEchoInput
	lastTime     *types.TimestampsInput
	panicOnEmpty bool
	failOnEmpty  bool
}

func (s *testService) GetThing(ctx context.Context, input *types.GetThingInput) (*types.GetThingOutput, error) {
	if input.Id == nil || *input.Id == "missing" {
		return nil, &types.ThingNotFound{Message: ptr("missing"), Id: input.Id}
	}
	return &types.GetThingOutput{
		Thing:    &types.Thing{Id: input.Id, Name: ptr("A thing"), CreatedAt: ptr(protocolTime)},
		Version:  ptr("v1"),
		Tags:     types.StringList{"a", "b"},
		Metadata: types.StringMap{"owner": "alice"},
	}, nil
}

func (s *testService) SearchThings(ctx context.Context, input *types.SearchThingsInput) (*types.SearchThingsOutput, error) {
	s.mu.Lock()
	s.lastSearch = input
	s.mu.Unlock()
	return &types.SearchThingsOutput{
		Things: types.ThingList{{Id: ptr("one"), Name: ptr("One"), CreatedAt: ptr(protocolTime)}},
		Total:  ptr(int32(1)),
	}, nil
}

func (s *testService) CreateThing(ctx context.Context, input *types.CreateThingInput) (*types.CreateThingOutput, error) {
	s.mu.Lock()
	s.lastCreate = input
	s.mu.Unlock()
	return &types.CreateThingOutput{Thing: &types.Thing{Id: ptr("new"), Name: input.Name, Color: input.Color, CreatedAt: ptr(protocolTime)}}, nil
}

func (s *testService) UploadData(ctx context.Context, input *types.UploadDataInput) (*types.UploadDataOutput, error) {
	s.mu.Lock()
	s.lastUpload = input
	s.mu.Unlock()
	return &types.UploadDataOutput{Size: ptr(int64(len(input.Data)))}, nil
}

func (s *testService) DownloadData(ctx context.Context, input *types.DownloadDataInput) (*types.DownloadDataOutput, error) {
	return &types.DownloadDataOutput{Data: []byte("downloaded")}, nil
}

func (s *testService) GetTime(ctx context.Context, input *types.Unit) (*types.GetTimeOutput, error) {
	return &types.GetTimeOutput{Now: ptr(protocolTime)}, nil
}

func (s *testService) SetStatus(ctx context.Context, input *types.SetStatusInput) (*types.SetStatusOutput, error) {
	return &types.SetStatusOutput{Status: input.Code, Message: ptr("ok")}, nil
}

func (s *testService) HeaderEcho(ctx context.Context, input *types.HeaderEchoInput) (*types.HeaderEchoOutput, error) {
	s.mu.Lock()
	s.lastHeader = input
	s.mu.Unlock()
	return &types.HeaderEchoOutput{
		HeaderString: input.HeaderString,
		HeaderInt:    input.HeaderInt,
		HeaderBool:   input.HeaderBool,
		HeaderList:   input.HeaderList,
		Extra:        input.Extra,
	}, nil
}

func (s *testService) EchoTimestamps(ctx context.Context, input *types.TimestampsInput) (*types.TimestampsOutput, error) {
	s.mu.Lock()
	s.lastTime = input
	s.mu.Unlock()
	return &types.TimestampsOutput{
		EpochSeconds:      input.EpochSeconds,
		EpochMilliseconds: input.EpochMilliseconds,
		HttpDate:          input.HttpDate,
		DateTime:          input.DateTime,
		EpochShape:        input.EpochShape,
	}, nil
}

func (s *testService) PatchThing(ctx context.Context, input *types.PatchThingInput) (*types.PatchThingOutput, error) {
	return &types.PatchThingOutput{Thing: &types.Thing{Id: input.Id, Name: input.Name, Color: input.Color, CreatedAt: ptr(protocolTime)}}, nil
}

func (s *testService) UnionEcho(ctx context.Context, input *types.UnionEchoInput) (*types.UnionEchoOutput, error) {
	return &types.UnionEchoOutput{Choice: input.Choice, Choices: input.Choices}, nil
}

func (s *testService) EchoNumbers(ctx context.Context, input *types.EchoNumbersInput) (*types.EchoNumbersOutput, error) {
	return &types.EchoNumbersOutput{BigInteger: input.BigInteger, BigDecimal: input.BigDecimal, Document: input.Document}, nil
}

func (s *testService) Empty(ctx context.Context, input *types.Unit) (*types.Unit, error) {
	if s.panicOnEmpty {
		panic("boom")
	}
	if s.failOnEmpty {
		return nil, errors.New("boom")
	}
	return &types.Unit{}, nil
}

func (s *testService) FailClient(ctx context.Context, input *types.Unit) (*types.Unit, error) {
	return nil, &types.ThingNotFound{Message: ptr("gone"), Id: ptr("abc")}
}

func (s *testService) FailServer(ctx context.Context, input *types.Unit) (*types.Unit, error) {
	return nil, &types.ThingUnavailable{Message: ptr("down"), Reason: ptr("maintenance")}
}

func decodeProtocolError(t *testing.T, err error) *smithy.ProtocolError {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var protocolErr *smithy.ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("error %T is not a *smithy.ProtocolError: %v", err, err)
	}
	return protocolErr
}

func TestProtocolRoundTrip(t *testing.T) {
	svc := &testService{}
	server := httptest.NewServer(NewServer(svc))
	defer server.Close()

	recorder := &recordingTransport{base: server.Client().Transport}
	client := NewClient(server.URL,
		WithHTTPClient(&http.Client{Transport: recorder}),
		WithHeader("X-Static", "yes"),
	)
	ctx := context.Background()

	t.Run("path label and headers", func(t *testing.T) {
		out, err := client.GetThing(ctx, &types.GetThingInput{Id: ptr("abc"), TraceId: ptr("trace-1")})
		if err != nil {
			t.Fatalf("GetThing: %v", err)
		}
		req := recorder.last(t)
		if req.method != "GET" || req.path != "/things/abc" {
			t.Errorf("request = %s %s", req.method, req.path)
		}
		if got := req.header.Get("X-Trace-Id"); got != "trace-1" {
			t.Errorf("X-Trace-Id = %q", got)
		}
		if got := req.header.Get("X-Static"); got != "yes" {
			t.Errorf("X-Static = %q", got)
		}
		if len(req.body) != 0 {
			t.Errorf("unexpected request body %q", req.body)
		}
		if out.Thing == nil || out.Thing.Id == nil || *out.Thing.Id != "abc" {
			t.Fatalf("thing = %+v", out.Thing)
		}
		if out.Version == nil || *out.Version != "v1" {
			t.Errorf("version = %v", out.Version)
		}
		if len(out.Tags) != 2 || out.Tags[0] != "a" {
			t.Errorf("tags = %v", out.Tags)
		}
		if out.Metadata["owner"] != "alice" {
			t.Errorf("metadata = %v", out.Metadata)
		}
		if out.Status == nil || *out.Status != 200 {
			t.Errorf("status = %v", out.Status)
		}
	})

	t.Run("query parameters", func(t *testing.T) {
		out, err := client.SearchThings(ctx, &types.SearchThingsInput{
			Query:  ptr("foo"),
			Colors: types.ColorList{types.ColorRed, types.ColorBlue},
			Since:  ptr(protocolSeconds),
			Params: types.StringMap{"page": "2", "extra": "x"},
		})
		if err != nil {
			t.Fatalf("SearchThings: %v", err)
		}
		req := recorder.last(t)
		if req.method != "GET" || req.path != "/things" {
			t.Errorf("request = %s %s", req.method, req.path)
		}
		query, err := url.ParseQuery(req.rawQuery)
		if err != nil {
			t.Fatal(err)
		}
		if got := query.Get("q"); got != "foo" {
			t.Errorf("q = %q", got)
		}
		if got := query["color"]; len(got) != 2 || got[0] != "red" || got[1] != "blue" {
			t.Errorf("color = %v", got)
		}
		wantSince, err := smithy.FormatTimestamp(protocolSeconds, smithy.TimestampEpochSeconds)
		if err != nil {
			t.Fatal(err)
		}
		if got := query.Get("since"); got != wantSince {
			t.Errorf("since = %q, want %q", got, wantSince)
		}
		if got := query.Get("page"); got != "2" {
			t.Errorf("page = %q", got)
		}
		if got := query.Get("extra"); got != "x" {
			t.Errorf("extra = %q", got)
		}
		svc.mu.Lock()
		input := svc.lastSearch
		svc.mu.Unlock()
		if input == nil {
			t.Fatal("service did not receive input")
		}
		if input.Query == nil || *input.Query != "foo" {
			t.Errorf("service query = %v", input.Query)
		}
		if len(input.Colors) != 2 || input.Colors[0] != types.ColorRed {
			t.Errorf("service colors = %v", input.Colors)
		}
		if input.Since == nil || !input.Since.Equal(protocolSeconds) {
			t.Errorf("service since = %v, want %v", input.Since, protocolSeconds)
		}
		if input.Params["page"] != "2" || input.Params["extra"] != "x" {
			t.Errorf("service params = %v", input.Params)
		}
		if _, exists := input.Params["q"]; exists {
			t.Errorf("@httpQueryParams contains the bound query parameter q: %v", input.Params)
		}
		if _, exists := input.Params["color"]; exists {
			t.Errorf("@httpQueryParams contains the bound query parameter color: %v", input.Params)
		}
		if len(out.Things) != 1 || out.Total == nil || *out.Total != 1 {
			t.Errorf("output = %+v", out)
		}
	})

	t.Run("JSON body", func(t *testing.T) {
		out, err := client.CreateThing(ctx, &types.CreateThingInput{Name: ptr("New thing"), Color: ptr(types.ColorGreen)})
		if err != nil {
			t.Fatalf("CreateThing: %v", err)
		}
		req := recorder.last(t)
		if req.method != "POST" || req.path != "/things" {
			t.Errorf("request = %s %s", req.method, req.path)
		}
		if got := req.header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if !strings.Contains(string(req.body), ` + "`" + `"display_name"` + "`" + `) {
			t.Errorf("request body does not use @jsonName: %s", req.body)
		}
		if strings.Contains(string(req.body), ` + "`" + `"name":` + "`" + `) {
			t.Errorf("request body uses the Smithy member name instead of @jsonName: %s", req.body)
		}
		var decoded types.CreateThingInput
		if err := json.Unmarshal(req.body, &decoded); err != nil {
			t.Fatalf("request body %q: %v", req.body, err)
		}
		if decoded.Name == nil || *decoded.Name != "New thing" {
			t.Errorf("decoded body = %+v", decoded)
		}
		if out.Thing == nil || out.Thing.Name == nil || *out.Thing.Name != "New thing" {
			t.Errorf("output = %+v", out.Thing)
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		_, err := client.CreateThing(ctx, &types.CreateThingInput{})
		protocolErr := decodeProtocolError(t, err)
		if protocolErr.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", protocolErr.StatusCode)
		}
		if protocolErr.Code != "ValidationException" {
			t.Errorf("code = %q", protocolErr.Code)
		}
		var envelope struct {
			Errors []struct {
				Path    string ` + "`json:\"path\"`" + `
				Message string ` + "`json:\"message\"`" + `
			} ` + "`json:\"errors\"`" + `
		}
		if err := json.Unmarshal(protocolErr.Body, &envelope); err != nil {
			t.Fatalf("validation body %q: %v", protocolErr.Body, err)
		}
		if len(envelope.Errors) != 1 || envelope.Errors[0].Path != "name" {
			t.Errorf("validation errors = %+v", envelope.Errors)
		}
	})

	t.Run("body excludes bound members", func(t *testing.T) {
		out, err := client.PatchThing(ctx, &types.PatchThingInput{Id: ptr("abc"), TraceId: ptr("trace-2"), Name: ptr("Patched")})
		if err != nil {
			t.Fatalf("PatchThing: %v", err)
		}
		req := recorder.last(t)
		if req.method != "PATCH" || req.path != "/things/abc" {
			t.Errorf("request = %s %s", req.method, req.path)
		}
		if got := req.header.Get("X-Trace-Id"); got != "trace-2" {
			t.Errorf("X-Trace-Id = %q", got)
		}
		body := string(req.body)
		if strings.Contains(body, ` + "`" + `"id"` + "`" + `) {
			t.Errorf("@httpLabel member leaked into the body: %s", body)
		}
		if strings.Contains(body, ` + "`" + `"traceId"` + "`" + `) {
			t.Errorf("@httpHeader member leaked into the body: %s", body)
		}
		if !strings.Contains(body, ` + "`" + `"name":"Patched"` + "`" + `) {
			t.Errorf("body is missing the unbound member: %s", body)
		}
		if out.Thing == nil || out.Thing.Name == nil || *out.Thing.Name != "Patched" {
			t.Errorf("output = %+v", out.Thing)
		}
	})

	t.Run("blob payload", func(t *testing.T) {
		out, err := client.UploadData(ctx, &types.UploadDataInput{Id: ptr("abc"), Data: []byte("hello")})
		if err != nil {
			t.Fatalf("UploadData: %v", err)
		}
		req := recorder.last(t)
		if req.method != "PUT" || req.path != "/things/abc/data" {
			t.Errorf("request = %s %s", req.method, req.path)
		}
		if got := req.header.Get("Content-Type"); got != "application/x-custom" {
			t.Errorf("Content-Type = %q", got)
		}
		if string(req.body) != "hello" {
			t.Errorf("body = %q", req.body)
		}
		if out.Size == nil || *out.Size != 5 {
			t.Errorf("size = %v", out.Size)
		}

		download, err := client.DownloadData(ctx, &types.DownloadDataInput{Id: ptr("abc")})
		if err != nil {
			t.Fatalf("DownloadData: %v", err)
		}
		if string(download.Data) != "downloaded" {
			t.Errorf("payload = %q", download.Data)
		}
		resp, err := http.Get(server.URL + "/things/abc/data")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if got := resp.Header.Get("Content-Type"); got != "application/x-custom" {
			t.Errorf("download Content-Type = %q", got)
		}
	})

	t.Run("response code override", func(t *testing.T) {
		out, err := client.SetStatus(ctx, &types.SetStatusInput{Code: ptr(int32(202))})
		if err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		if out.Status == nil || *out.Status != 202 {
			t.Errorf("status = %v", out.Status)
		}
		if out.Message == nil || *out.Message != "ok" {
			t.Errorf("message = %v", out.Message)
		}
	})

	t.Run("headers and prefix headers", func(t *testing.T) {
		out, err := client.HeaderEcho(ctx, &types.HeaderEchoInput{
			HeaderString: ptr("s"),
			HeaderInt:    ptr(int32(7)),
			HeaderBool:   ptr(true),
			HeaderList:   types.StringList{"one", "two"},
			Extra:        types.StringMap{"k": "v"},
		})
		if err != nil {
			t.Fatalf("HeaderEcho: %v", err)
		}
		req := recorder.last(t)
		if len(req.body) != 0 {
			t.Errorf("unexpected body %q", req.body)
		}
		if got := req.header.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q, want no content type", got)
		}
		if got := req.header.Get("X-String"); got != "s" {
			t.Errorf("X-String = %q", got)
		}
		if got := req.header.Get("X-Int"); got != "7" {
			t.Errorf("X-Int = %q", got)
		}
		if got := req.header.Get("X-Bool"); got != "true" {
			t.Errorf("X-Bool = %q", got)
		}
		if got := req.header.Get("X-List"); got != "one, two" {
			t.Errorf("X-List = %q", got)
		}
		if got := req.header.Get("X-Extra-K"); got != "v" {
			t.Errorf("X-Extra-K = %q", got)
		}
		if out.HeaderString == nil || *out.HeaderString != "s" {
			t.Errorf("output string = %v", out.HeaderString)
		}
		if out.HeaderInt == nil || *out.HeaderInt != 7 {
			t.Errorf("output int = %v", out.HeaderInt)
		}
		if out.HeaderBool == nil || !*out.HeaderBool {
			t.Errorf("output bool = %v", out.HeaderBool)
		}
		if len(out.HeaderList) != 2 || out.HeaderList[1] != "two" {
			t.Errorf("output list = %v", out.HeaderList)
		}
		if out.Extra["k"] != "v" {
			t.Errorf("output prefix headers = %v", out.Extra)
		}
	})

	t.Run("timestamp formats", func(t *testing.T) {
		out, err := client.EchoTimestamps(ctx, &types.TimestampsInput{
			EpochSeconds:      ptr(protocolSeconds),
			EpochMilliseconds: ptr(protocolMillis),
			HttpDate:          ptr(protocolHTTPDate),
			DateTime:          ptr(protocolDateTime),
			EpochShape:        ptr(protocolSeconds),
		})
		if err != nil {
			t.Fatalf("EchoTimestamps: %v", err)
		}
		if out.EpochSeconds == nil || !out.EpochSeconds.Equal(protocolSeconds) {
			t.Errorf("epoch-seconds = %v, want %v", out.EpochSeconds, protocolSeconds)
		}
		if out.EpochMilliseconds == nil || !out.EpochMilliseconds.Equal(protocolMillis) {
			t.Errorf("epoch-milliseconds = %v, want %v", out.EpochMilliseconds, protocolMillis)
		}
		if out.HttpDate == nil || !out.HttpDate.Equal(protocolHTTPDate) {
			t.Errorf("http-date = %v, want %v", out.HttpDate, protocolHTTPDate)
		}
		if out.DateTime == nil || !out.DateTime.Equal(protocolDateTime) {
			t.Errorf("date-time = %v, want %v", out.DateTime, protocolDateTime)
		}
		if out.EpochShape == nil || !out.EpochShape.Equal(protocolSeconds) {
			t.Errorf("shape timestampFormat = %v, want %v", out.EpochShape, protocolSeconds)
		}
	})

	t.Run("empty input and output", func(t *testing.T) {
		out, err := client.Empty(ctx, &types.Unit{})
		if err != nil {
			t.Fatalf("Empty: %v", err)
		}
		if out == nil {
			t.Fatal("Empty returned nil output")
		}
		req := recorder.last(t)
		if req.method != "POST" || req.path != "/empty" {
			t.Errorf("request = %s %s", req.method, req.path)
		}
		if len(req.body) != 0 {
			t.Errorf("unexpected body %q", req.body)
		}
	})

	t.Run("union values", func(t *testing.T) {
		thing := &types.Thing{Id: ptr("u1"), Name: ptr("Union thing"), CreatedAt: ptr(protocolTime)}
		out, err := client.UnionEcho(ctx, &types.UnionEchoInput{
			Choice: &types.ThingChoiceName{Value: "chosen"},
			Choices: types.ThingChoiceList{
				&types.ThingChoiceName{Value: "first"},
				nil,
				&types.ThingChoiceThing{Value: thing},
			},
		})
		if err != nil {
			t.Fatalf("UnionEcho: %v", err)
		}
		choice, ok := out.Choice.(*types.ThingChoiceName)
		if !ok {
			t.Fatalf("choice = %T, want *types.ThingChoiceName", out.Choice)
		}
		if choice.Value != "chosen" {
			t.Errorf("choice value = %q", choice.Value)
		}
		if len(out.Choices) != 3 {
			t.Fatalf("choices = %v", out.Choices)
		}
		if _, ok := out.Choices[0].(*types.ThingChoiceName); !ok {
			t.Errorf("choices[0] = %T", out.Choices[0])
		}
		if out.Choices[1] != nil {
			t.Errorf("choices[1] = %v, want nil", out.Choices[1])
		}
		unionThing, ok := out.Choices[2].(*types.ThingChoiceThing)
		if !ok || unionThing.Value == nil || unionThing.Value.Id == nil || *unionThing.Value.Id != "u1" {
			t.Errorf("choices[2] = %+v", out.Choices[2])
		}
	})

	t.Run("big numbers", func(t *testing.T) {
		integer, ok := new(big.Int).SetString("123456789012345678901234567890", 10)
		if !ok {
			t.Fatal("invalid big integer literal")
		}
		bigDecimal := types.MustDecimal("12345.678901234567890123456789")
		document := map[string]any{"big": json.Number("12345678901234567890")}
		out, err := client.EchoNumbers(ctx, &types.EchoNumbersInput{BigInteger: integer, BigDecimal: &bigDecimal, Document: document})
		if err != nil {
			t.Fatalf("EchoNumbers: %v", err)
		}
		if out.BigInteger == nil || out.BigInteger.Cmp(integer) != 0 {
			t.Errorf("bigInteger = %v, want %v", out.BigInteger, integer)
		}
		if out.BigDecimal == nil || out.BigDecimal.String() != bigDecimal.String() {
			t.Errorf("bigDecimal = %v, want %v", out.BigDecimal, bigDecimal)
		}
		roundTripped, ok := out.Document.(map[string]any)
		if !ok {
			t.Fatalf("document = %T, want map[string]any", out.Document)
		}
		number, ok := roundTripped["big"].(json.Number)
		if !ok {
			t.Fatalf("document.big = %T, want json.Number", roundTripped["big"])
		}
		if number.String() != "12345678901234567890" {
			t.Errorf("document.big = %q", number.String())
		}
	})

	t.Run("per-call options", func(t *testing.T) {
		_, err := client.GetTime(ctx, &types.Unit{}, func(options *Options) {
			options.Headers.Set("X-Call", "1")
		})
		if err != nil {
			t.Fatalf("GetTime: %v", err)
		}
		req := recorder.last(t)
		if got := req.header.Get("X-Call"); got != "1" {
			t.Errorf("X-Call = %q", got)
		}
	})

	t.Run("modeled errors", func(t *testing.T) {
		_, err := client.FailClient(ctx, &types.Unit{})
		var notFound *types.ThingNotFound
		if !errors.As(err, &notFound) {
			t.Fatalf("FailClient error = %T %v, want *types.ThingNotFound", err, err)
		}
		if notFound.ErrorCode() != "ThingNotFound" {
			t.Errorf("code = %q", notFound.ErrorCode())
		}
		if notFound.Message == nil || *notFound.Message != "gone" {
			t.Errorf("message = %v", notFound.Message)
		}
		if notFound.Id == nil || *notFound.Id != "abc" {
			t.Errorf("id = %v", notFound.Id)
		}

		_, err = client.FailServer(ctx, &types.Unit{})
		var unavailable *types.ThingUnavailable
		if !errors.As(err, &unavailable) {
			t.Fatalf("FailServer error = %T %v, want *types.ThingUnavailable", err, err)
		}
		if unavailable.ErrorFault() != "server" {
			t.Errorf("fault = %q", unavailable.ErrorFault())
		}
		if unavailable.Reason == nil || *unavailable.Reason != "maintenance" {
			t.Errorf("reason = %v", unavailable.Reason)
		}
	})

	t.Run("unknown error becomes ProtocolError", func(t *testing.T) {
		unknown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTeapot)
			_, _ = io.WriteString(w, ` + "`" + `{"code":"Teapot","message":"short and stout"}` + "`" + `)
		}))
		defer unknown.Close()
		unknownClient := NewClient(unknown.URL)
		_, err := unknownClient.GetThing(ctx, &types.GetThingInput{Id: ptr("abc")})
		protocolErr := decodeProtocolError(t, err)
		if protocolErr.StatusCode != http.StatusTeapot {
			t.Errorf("status = %d", protocolErr.StatusCode)
		}
		if protocolErr.Code != "Teapot" || protocolErr.Message != "short and stout" {
			t.Errorf("error = %+v", protocolErr)
		}
		if !strings.Contains(string(protocolErr.Body), "Teapot") {
			t.Errorf("body = %q", protocolErr.Body)
		}
	})
}

func TestProtocolServerErrors(t *testing.T) {
	server := httptest.NewServer(NewServer(&testService{}))
	defer server.Close()

	t.Run("unknown route", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/nope")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), ` + "`" + `"code":"NotFound"` + "`" + `) {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		resp, err := http.Post(server.URL+"/time", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("status = %d", resp.StatusCode)
		}
		if allow := resp.Header.Get("Allow"); allow != "GET" {
			t.Errorf("Allow = %q", allow)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), ` + "`" + `"code":"MethodNotAllowed"` + "`" + `) {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		resp, err := http.Post(server.URL+"/things", "application/json", strings.NewReader("{bad"))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), ` + "`" + `"code":"BadRequest"` + "`" + `) {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("body too large", func(t *testing.T) {
		limited := httptest.NewServer(NewServer(&testService{}, WithMaxBodyBytes(8)))
		defer limited.Close()
		resp, err := http.Post(limited.URL+"/things", "application/json", strings.NewReader(` + "`" + `{"name":"a very long name"}` + "`" + `))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), ` + "`" + `"code":"PayloadTooLarge"` + "`" + `) {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("unhandled error becomes InternalFailure", func(t *testing.T) {
		failing := httptest.NewServer(NewServer(&testService{failOnEmpty: true}))
		defer failing.Close()
		client := NewClient(failing.URL)
		_, err := client.Empty(context.Background(), &types.Unit{})
		protocolErr := decodeProtocolError(t, err)
		if protocolErr.StatusCode != http.StatusInternalServerError || protocolErr.Code != "InternalFailure" {
			t.Errorf("error = %+v", protocolErr)
		}
	})

	t.Run("panic becomes InternalFailure", func(t *testing.T) {
		panicking := &testService{panicOnEmpty: true}
		panickingServer := httptest.NewServer(NewServer(panicking))
		defer panickingServer.Close()
		client := NewClient(panickingServer.URL)
		_, err := client.Empty(context.Background(), &types.Unit{})
		protocolErr := decodeProtocolError(t, err)
		if protocolErr.StatusCode != http.StatusInternalServerError || protocolErr.Code != "InternalFailure" {
			t.Errorf("error = %+v", protocolErr)
		}
	})

	t.Run("modeled error statuses", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/fail/client")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("client error status = %d, want 404", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		for _, want := range []string{` + "`" + `"code":"ThingNotFound"` + "`" + `, ` + "`" + `"message":"gone"` + "`" + `, ` + "`" + `"id":"abc"` + "`" + `} {
			if !strings.Contains(string(body), want) {
				t.Errorf("client error body %s does not contain %s", body, want)
			}
		}

		resp, err = http.Get(server.URL + "/fail/server")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("server error status = %d, want 503", resp.StatusCode)
		}
		body, _ = io.ReadAll(resp.Body)
		for _, want := range []string{` + "`" + `"code":"ThingUnavailable"` + "`" + `, ` + "`" + `"message":"down"` + "`" + `, ` + "`" + `"reason":"maintenance"` + "`" + `} {
			if !strings.Contains(string(body), want) {
				t.Errorf("server error body %s does not contain %s", body, want)
			}
		}
	})

	t.Run("trailing slash", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/time/")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})

	t.Run("base path", func(t *testing.T) {
		mounted := httptest.NewServer(NewServer(&testService{}, WithServerBasePath("/api")))
		defer mounted.Close()
		client := NewClient(mounted.URL, WithBasePath("/api"))
		out, err := client.GetTime(context.Background(), &types.Unit{})
		if err != nil {
			t.Fatalf("GetTime: %v", err)
		}
		if out.Now == nil || !out.Now.Equal(protocolTime) {
			t.Errorf("now = %v", out.Now)
		}
		resp, err := http.Get(mounted.URL + "/time")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("unmounted status = %d", resp.StatusCode)
		}
	})
}
`

func TestProtocolServiceTraitMismatch(t *testing.T) {
	m := assembleProtocolSources(t, `$version: "2.0"
namespace example.other

@protocolDefinition
@trait(selector: "service")
structure otherProtocol {}
`, `$version: "2.0"
namespace example.mismatch

use example.other#otherProtocol

@otherProtocol
service Mismatch {
    operations: [Op]
}

@http(method: "GET", uri: "/op")
operation Op {}
`)
	_, diags, err := runProtocolGenerator(t, m, model.MustShapeID("example.mismatch#Mismatch"), "example.com/mismatch")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := false
	for _, d := range diags {
		if d.Severity == validation.Danger && strings.Contains(d.Message, "uses protocol") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %v, want protocol mismatch", diags)
	}
}

func TestProtocolGeneratedOutputIsDeterministic(t *testing.T) {
	m := loadProtocolFixture(t, "../../testdata/http.smithy")
	first, _, err := runProtocolGenerator(t, m, model.MustShapeID("example.http#HttpJson"), "example.com/http")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		next, _, err := runProtocolGenerator(t, m, model.MustShapeID("example.http#HttpJson"), "example.com/http")
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != len(next) {
			t.Fatalf("file count changed between runs: %d vs %d", len(first), len(next))
		}
		for j := range first {
			if first[j].Path != next[j].Path || string(first[j].Content) != string(next[j].Content) {
				t.Fatalf("output changed between runs for %s", first[j].Path)
			}
		}
	}
}

func TestProtocolGeneratedGoModHasNoDependencies(t *testing.T) {
	m := loadProtocolFixture(t, "../../testdata/http.smithy")
	files, _, err := runProtocolGenerator(t, m, model.MustShapeID("example.http#HttpJson"), "example.com/http")
	if err != nil {
		t.Fatal(err)
	}
	var gomod string
	for _, file := range files {
		if file.Path == "go.mod" {
			gomod = string(file.Content)
		}
		if strings.Contains(strings.ToLower(string(file.Content)), "aws") {
			t.Errorf("%s contains an AWS reference", file.Path)
		}
	}
	if gomod == "" {
		t.Fatal("generated module has no go.mod")
	}
	if strings.Contains(gomod, "require") {
		t.Errorf("go.mod has dependencies:\n%s", gomod)
	}
}

func TestProtocolEmptyServiceCompiles(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	m := assembleProtocolSources(t, `$version: "2.0"
namespace example.empty

use gosmithy.protocols#httpJson

@httpJson
service EmptyService {}
`)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("example.empty#EmptyService"), "example.com/empty")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, d := range diags {
		if d.Severity == validation.Danger {
			t.Fatalf("unexpected DANGER diagnostic: %s", d)
		}
	}
	dir := t.TempDir()
	writeFiles(t, dir, files)
	runGo(t, dir, "build", "./...")
	runGo(t, dir, "vet", "./...")
}

func TestProtocolBlobOnlyServiceCompiles(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	m := assembleProtocolSources(t, `$version: "2.0"
namespace example.blob

use gosmithy.protocols#httpJson

@httpJson
service BlobService {
    operations: [PutBlob]
}

structure PutBlobInput {
    @httpPayload
    data: Blob
}

@http(method: "PUT", uri: "/blob")
operation PutBlob {
    input: PutBlobInput
}
`)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("example.blob#BlobService"), "example.com/blob")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, d := range diags {
		if d.Severity == validation.Danger {
			t.Fatalf("unexpected DANGER diagnostic: %s", d)
		}
	}
	dir := t.TempDir()
	writeFiles(t, dir, files)
	runGo(t, dir, "build", "./...")
	runGo(t, dir, "vet", "./...")
}
