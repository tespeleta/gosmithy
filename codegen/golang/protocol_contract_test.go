package golang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tespeleta/gosmithy/model"
)

type contractCase struct {
	name   string
	source string
	run    string
	test   string
}

// protocolContracts assert wire behavior with hand-written requests or a fake
// transport. They never use the generated client against the generated server
// as the oracle.
var protocolContracts = []contractCase{
	{
		name: "union_payload_input",
		source: contractModel(`union U {
    name: String
    id: Integer
}

structure In {
    @required
    @httpPayload
    u: U
}`, contractOpInput),
		run: "^TestUnionPayloadInput$",
		test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type unionInputService struct{ got types.U }

func (s *unionInputService) Op(ctx context.Context, in *types.In) (*types.Unit, error) {
	s.got = in.U
	return &types.Unit{}, nil
}

func TestUnionPayloadInput(t *testing.T) {
	s := &unionInputService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"name":"x"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := s.got.(*types.UName); !ok {
		t.Fatalf("decoded union = %T, want *types.UName", s.got)
	}
}
`,
	},
	{
		name: "union_payload_output",
		source: contractModel(`union U {
    name: String
    id: Integer
}

structure Out {
    @required
    @httpPayload
    u: U
}`, contractOpOutput),
		run: "^TestUnionPayloadOutput$",
		test: `package contract

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/contract/types"
)

type unionRoundTrip func(*http.Request) (*http.Response, error)

func (f unionRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUnionPayloadOutput(t *testing.T) {
	client := NewClient("http://example.com", WithHTTPClient(&http.Client{Transport: unionRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(` + "`" + `{"name":"x"}` + "`" + `)),
			Request:    r,
		}, nil
	})}))
	out, err := client.Op(context.Background(), &types.Unit{})
	if err != nil {
		t.Fatalf("Op: %v", err)
	}
	if _, ok := out.U.(*types.UName); !ok {
		t.Fatalf("decoded union = %T, want *types.UName", out.U)
	}
}
`,
	},
	{
		name: "timestamp_payload_epoch",
		source: contractModel(`structure In {
    @required
    @httpPayload
    @timestampFormat("epoch-seconds")
    when: Timestamp
}

structure Out {
    @required
    @httpPayload
    @timestampFormat("epoch-seconds")
    when: Timestamp
}`, contractOpInOut),
		run: "^TestTimestampPayloadEpoch$",
		test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/contract/types"
)

type timestampService struct{ got *time.Time }

func (s *timestampService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	s.got = in.When
	return &types.Out{When: in.When}, nil
}

func TestTimestampPayloadEpoch(t *testing.T) {
	s := &timestampService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `"1706933106.123"` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := time.Unix(1706933106, 123000000).UTC()
	if s.got == nil || !s.got.Equal(want) {
		t.Fatalf("decoded = %v, want %v", s.got, want)
	}
	if got, wantBody := rec.Body.String(), ` + "`" + `"1706933106.123"` + "`" + `; got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}
`,
	},
	{
		name: "timestamp_payload_datetime_control",
		source: contractModel(`structure In {
    @required
    @httpPayload
    @timestampFormat("date-time")
    when: Timestamp
}

structure Out {
    @required
    @httpPayload
    @timestampFormat("date-time")
    when: Timestamp
}`, contractOpInOut),
		run: "^TestTimestampPayloadDateTime$",
		test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/contract/types"
)

type dateTimeService struct{ got *time.Time }

func (s *dateTimeService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	s.got = in.When
	return &types.Out{When: in.When}, nil
}

func TestTimestampPayloadDateTime(t *testing.T) {
	s := &dateTimeService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `"2024-02-03T04:05:06Z"` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	if s.got == nil || !s.got.Equal(want) {
		t.Fatalf("decoded = %v, want %v", s.got, want)
	}
	if got, wantBody := rec.Body.String(), ` + "`" + `"2024-02-03T04:05:06Z"` + "`" + `; got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}
`,
	},
	{
		name: "timestamp_list_query",
		source: contractModel(`list Times {
    @timestampFormat("epoch-seconds")
    member: Timestamp
}

structure In {
    @httpQuery("t")
    times: Times
}`, contractOpInput),
		run: "^TestTimestampListQuery",
		test: `package contract

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

type listRoundTrip func(*http.Request) (*http.Response, error)

func (f listRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTimestampListQueryClient(t *testing.T) {
	var gotQuery string
	client := NewClient("http://example.com", WithHTTPClient(&http.Client{Transport: listRoundTrip(func(r *http.Request) (*http.Response, error) {
		gotQuery = r.URL.RawQuery
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})}))
	_, err := client.Op(context.Background(), &types.In{Times: types.Times{time.Unix(1706933106, 0).UTC()}})
	if err != nil {
		t.Fatalf("Op: %v", err)
	}
	if gotQuery != "t=1706933106" {
		t.Fatalf("query = %q, want %q", gotQuery, "t=1706933106")
	}
}

type listService struct{ got *types.In }

func (s *listService) Op(ctx context.Context, in *types.In) (*types.Unit, error) {
	s.got = in
	return &types.Unit{}, nil
}

func TestTimestampListQueryServer(t *testing.T) {
	s := &listService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/op?t=1706933106", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if s.got == nil || len(s.got.Times) != 1 {
		t.Fatalf("decoded input = %+v", s.got)
	}
	want := time.Unix(1706933106, 0).UTC()
	if !s.got.Times[0].Equal(want) {
		t.Fatalf("decoded time = %v, want %v", s.got.Times[0], want)
	}
}
`,
	},
}

func TestProtocolContracts(t *testing.T) {
	requireIntegration(t)
	for _, tc := range protocolContracts {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := assembleSources(t, tc.source)
			files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			requireNoGeneratorDanger(t, diags)
			dir := t.TempDir()
			writeFiles(t, dir, files)
			if err := os.WriteFile(filepath.Join(dir, "wire_test.go"), []byte(tc.test), 0o644); err != nil {
				t.Fatalf("write wire_test.go: %v", err)
			}
			runGo(t, dir, "test", "-run", tc.run, ".")
		})
	}
}

func contractModel(shapes, op string) string {
	return `$version: "2.0"
namespace tdd

service S {
    version: "1.0.0"
    operations: [Op]
}

` + shapes + "\n\n" + op + "\n"
}

const (
	contractOpInput = `@http(method: "POST", uri: "/op")
operation Op {
    input: In
}`

	contractOpOutput = `@http(method: "POST", uri: "/op")
operation Op {
    output: Out
}`

	contractOpInOut = `@http(method: "POST", uri: "/op")
operation Op {
    input: In
    output: Out
}`
)
