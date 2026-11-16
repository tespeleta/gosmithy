package golang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tespeleta/gosmithy/model"
)

// protocolContracts assert wire behavior with hand-written requests or a fake
// transport. They never use the generated client against the generated server
// as the oracle. All contracts share one generated module so the per-case cost
// is code generation plus a single compile, not one module per contract.
func TestProtocolContracts(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	m := assembleSources(t, contractModel)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	requireNoGeneratorDanger(t, diags)
	dir := t.TempDir()
	writeFiles(t, dir, files)
	if err := os.WriteFile(filepath.Join(dir, "wire_test.go"), []byte(contractTests), 0o644); err != nil {
		t.Fatalf("write wire_test.go: %v", err)
	}
	runGo(t, dir, "test", ".")
}

const contractModel = `$version: "2.0"
namespace tdd

service S {
    version: "1.0.0"
    operations: [
        UnionInput
        UnionOutput
        TimestampEpoch
        TimestampDateTime
        TimestampList
    ]
}

union U {
    name: String
    id: Integer
}

structure UnionInputIn {
    @required
    @httpPayload
    u: U
}

@http(method: "POST", uri: "/union-input")
operation UnionInput {
    input: UnionInputIn
}

structure UnionOutputOut {
    @required
    @httpPayload
    u: U
}

@http(method: "POST", uri: "/union-output")
operation UnionOutput {
    output: UnionOutputOut
}

structure TimestampEpochIn {
    @required
    @httpPayload
    @timestampFormat("epoch-seconds")
    when: Timestamp
}

structure TimestampEpochOut {
    @required
    @httpPayload
    @timestampFormat("epoch-seconds")
    when: Timestamp
}

@http(method: "POST", uri: "/timestamp-epoch")
operation TimestampEpoch {
    input: TimestampEpochIn
    output: TimestampEpochOut
}

structure TimestampDateTimeIn {
    @required
    @httpPayload
    @timestampFormat("date-time")
    when: Timestamp
}

structure TimestampDateTimeOut {
    @required
    @httpPayload
    @timestampFormat("date-time")
    when: Timestamp
}

@http(method: "POST", uri: "/timestamp-datetime")
operation TimestampDateTime {
    input: TimestampDateTimeIn
    output: TimestampDateTimeOut
}

list Times {
    @timestampFormat("epoch-seconds")
    member: Timestamp
}

structure TimestampListIn {
    @httpQuery("t")
    times: Times
}

@http(method: "POST", uri: "/timestamp-list")
operation TimestampList {
    input: TimestampListIn
}
`

const contractTests = `package contract

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/contract/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func withTransport(fn roundTripFunc) *Client {
	return NewClient("http://example.com", WithHTTPClient(&http.Client{Transport: fn}))
}

func wireResponse(r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

type contractService struct {
	union types.U
	when  *time.Time
	list  *types.TimestampListIn
}

func (s *contractService) UnionInput(ctx context.Context, in *types.UnionInputIn) (*types.Unit, error) {
	s.union = in.U
	return &types.Unit{}, nil
}

func (s *contractService) UnionOutput(ctx context.Context, in *types.Unit) (*types.UnionOutputOut, error) {
	return &types.UnionOutputOut{U: s.union}, nil
}

func (s *contractService) TimestampEpoch(ctx context.Context, in *types.TimestampEpochIn) (*types.TimestampEpochOut, error) {
	s.when = in.When
	return &types.TimestampEpochOut{When: in.When}, nil
}

func (s *contractService) TimestampDateTime(ctx context.Context, in *types.TimestampDateTimeIn) (*types.TimestampDateTimeOut, error) {
	s.when = in.When
	return &types.TimestampDateTimeOut{When: in.When}, nil
}

func (s *contractService) TimestampList(ctx context.Context, in *types.TimestampListIn) (*types.Unit, error) {
	s.list = in
	return &types.Unit{}, nil
}

func TestUnionPayloadInput(t *testing.T) {
	s := &contractService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/union-input", strings.NewReader(` + "`" + `{"name":"x"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := s.union.(*types.UName); !ok {
		t.Fatalf("decoded union = %T, want *types.UName", s.union)
	}
}

func TestUnionPayloadOutput(t *testing.T) {
	client := withTransport(func(r *http.Request) (*http.Response, error) {
		return wireResponse(r, ` + "`" + `{"name":"x"}` + "`" + `), nil
	})
	out, err := client.UnionOutput(context.Background(), &types.Unit{})
	if err != nil {
		t.Fatalf("UnionOutput: %v", err)
	}
	if _, ok := out.U.(*types.UName); !ok {
		t.Fatalf("decoded union = %T, want *types.UName", out.U)
	}
}

func TestTimestampPayloadEpoch(t *testing.T) {
	s := &contractService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/timestamp-epoch", strings.NewReader(` + "`" + `"1706933106.123"` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := time.Unix(1706933106, 123000000).UTC()
	if s.when == nil || !s.when.Equal(want) {
		t.Fatalf("decoded = %v, want %v", s.when, want)
	}
	if got, wantBody := rec.Body.String(), ` + "`" + `"1706933106.123"` + "`" + `; got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

func TestTimestampPayloadDateTime(t *testing.T) {
	s := &contractService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/timestamp-datetime", strings.NewReader(` + "`" + `"2024-02-03T04:05:06Z"` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	if s.when == nil || !s.when.Equal(want) {
		t.Fatalf("decoded = %v, want %v", s.when, want)
	}
	if got, wantBody := rec.Body.String(), ` + "`" + `"2024-02-03T04:05:06Z"` + "`" + `; got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

func TestTimestampListQueryClient(t *testing.T) {
	var gotQuery string
	client := withTransport(func(r *http.Request) (*http.Response, error) {
		gotQuery = r.URL.RawQuery
		return wireResponse(r, ""), nil
	})
	if _, err := client.TimestampList(context.Background(), &types.TimestampListIn{Times: types.Times{time.Unix(1706933106, 0).UTC()}}); err != nil {
		t.Fatalf("TimestampList: %v", err)
	}
	if gotQuery != "t=1706933106" {
		t.Fatalf("query = %q, want %q", gotQuery, "t=1706933106")
	}
}

func TestTimestampListQueryServer(t *testing.T) {
	s := &contractService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/timestamp-list?t=1706933106", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if s.list == nil || len(s.list.Times) != 1 {
		t.Fatalf("decoded input = %+v", s.list)
	}
	want := time.Unix(1706933106, 0).UTC()
	if !s.list.Times[0].Equal(want) {
		t.Fatalf("decoded time = %v, want %v", s.list.Times[0], want)
	}
}
`
