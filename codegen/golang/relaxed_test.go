package golang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/validation"
)

func TestRelaxedContracts(t *testing.T) {
	cases := []struct {
		name   string
		source string
		run    string
		test   string
	}{
		{
			name: "service_relaxed_scalars",
			source: relaxedSource(`@relaxedInput`, `structure In {
    ok: Boolean
    count: Integer
    ratio: Double
    amount: BigDecimal
}`, `structure Out {
    ok: Boolean
    count: Integer
    ratio: Double
    amount: BigDecimal
}`),
			run: "^TestServiceRelaxed$",
			test: `package contract

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type scalarService struct{}

func boolValue(v *types.RelaxedBool) *bool {
	if v == nil {
		return nil
	}
	b := v.Value()
	return &b
}

func (s *scalarService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	return &types.Out{Ok: boolValue(in.Ok), Count: in.Count, Ratio: in.Ratio, Amount: in.Amount}, nil
}

func TestServiceRelaxed(t *testing.T) {
	h := NewServer(&scalarService{})
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"ok":"true","count":"100","ratio":"1.5","amount":"100.50"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if string(out["ok"]) != "true" || string(out["count"]) != "100" || string(out["ratio"]) != "1.5" || string(out["amount"]) != "100.50" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
`,
		},
		{
			name: "strict_rejects_quoted_scalars",
			source: relaxedSource("", `structure In {
    ok: Boolean
    count: Integer
    amount: BigDecimal
}`, `structure Out {
    ok: Boolean
    count: Integer
    amount: BigDecimal
}`),
			run: "^TestStrictRejectsQuoted$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type strictService struct{}

func (s *strictService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	return &types.Out{Ok: in.Ok, Count: in.Count, Amount: in.Amount}, nil
}

func TestStrictRejectsQuoted(t *testing.T) {
	h := NewServer(&strictService{})
	for _, body := range []string{
		` + "`" + `{"ok":"true"}` + "`" + `,
		` + "`" + `{"count":"100"}` + "`" + `,
		` + "`" + `{"amount":"100.50"}` + "`" + `,
	} {
		req := httptest.NewRequest("POST", "/op", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Fatalf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"ok":true,"count":100,"amount":100.50}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("strict number: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
`,
		},
		{
			name: "member_opt_in",
			source: relaxedSource("", `structure In {
    @relaxedInput
    ok: Boolean
    count: Integer
}`, `structure Out {
    ok: Boolean
    count: Integer
}`),
			run: "^TestMemberOptIn$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type optInService struct{}

func relaxedBoolValue(v *types.RelaxedBool) *bool {
	if v == nil {
		return nil
	}
	b := v.Value()
	return &b
}

func (s *optInService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	return &types.Out{Ok: relaxedBoolValue(in.Ok), Count: in.Count}, nil
}

func TestMemberOptIn(t *testing.T) {
	h := NewServer(&optInService{})
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"ok":true,"count":"100"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("strict count: status = %d, want 400", rec.Code)
	}
	req = httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"ok":"true","count":100}` + "`" + `))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("relaxed bool: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
`,
		},
		{
			name: "member_opt_out",
			source: relaxedSource(`@relaxedInput`, `structure In {
    ok: Boolean
    @relaxedInput(enabled: false)
    count: Integer
}`, `structure Out {
    ok: Boolean
    count: Integer
}`),
			run: "^TestMemberOptOut$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type optOutService struct{}

func relaxedBoolValue(v *types.RelaxedBool) *bool {
	if v == nil {
		return nil
	}
	b := v.Value()
	return &b
}

func (s *optOutService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	return &types.Out{Ok: relaxedBoolValue(in.Ok), Count: in.Count}, nil
}

func TestMemberOptOut(t *testing.T) {
	h := NewServer(&optOutService{})
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"ok":"true","count":"100"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("strict count: status = %d, want 400", rec.Code)
	}
	req = httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"ok":"true","count":100}` + "`" + `))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("relaxed bool: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
`,
		},
		{
			name: "byte_range",
			source: relaxedSource(`@relaxedInput`, `structure In {
    count: Byte
}`, `structure Out {
    count: Byte
}`),
			run: "^TestByteRange$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type byteService struct{}

func (s *byteService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	return &types.Out{Count: in.Count}, nil
}

func TestByteRange(t *testing.T) {
	h := NewServer(&byteService{})
	for _, body := range []string{` + "`" + `{"count":"300"}` + "`" + `, ` + "`" + `{"count":"abc"}` + "`" + `} {
		req := httptest.NewRequest("POST", "/op", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Fatalf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"count":"127"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
`,
		},
		{
			name: "invalid_bool",
			source: relaxedSource(`@relaxedInput`, `structure In {
    ok: Boolean
}`, ""),
			run: "^TestInvalidBool$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type boolService struct{}

func (s *boolService) Op(ctx context.Context, in *types.In) (*types.Unit, error) {
	return &types.Unit{}, nil
}

func TestInvalidBool(t *testing.T) {
	h := NewServer(&boolService{})
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"ok":"yes"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
`,
		},
		{
			name: "nested_input",
			source: relaxedSource(`@relaxedInput`, `structure Inner {
    ok: Boolean
    count: Integer
}

structure In {
    inner: Inner
}`, ""),
			run: "^TestNestedInput$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type nestedService struct{ got *types.Inner }

func (s *nestedService) Op(ctx context.Context, in *types.In) (*types.Unit, error) {
	s.got = in.Inner
	return &types.Unit{}, nil
}

func TestNestedInput(t *testing.T) {
	s := &nestedService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"inner":{"ok":"true","count":"100"}}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if s.got == nil || s.got.Ok == nil || !bool(*s.got.Ok) || s.got.Count == nil || *s.got.Count != 100 {
		t.Fatalf("decoded = %+v", s.got)
	}
}
`,
		},
		{
			name: "payload_number",
			source: relaxedSource(`@relaxedInput`, `structure In {
    @httpPayload
    count: Integer
}`, ""),
			run: "^TestPayloadNumber$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type payloadService struct{ got *int32 }

func (s *payloadService) Op(ctx context.Context, in *types.In) (*types.Unit, error) {
	s.got = in.Count
	return &types.Unit{}, nil
}

func TestPayloadNumber(t *testing.T) {
	s := &payloadService{}
	h := NewServer(s)
	for _, body := range []string{` + "`" + `100` + "`" + `, ` + "`" + `"100"` + "`" + `} {
		req := httptest.NewRequest("POST", "/op", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("body %s: status = %d, body = %s", body, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `"abc"` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("invalid payload: status = %d, want 400", rec.Code)
	}
}
`,
		},
		{
			name: "relaxed_bool_omitempty",
			source: relaxedSource(`@relaxedInput`, `structure In {
    ok: Boolean
}`, ""),
			run: "^TestRelaxedBoolOmitempty$",
			test: `package contract

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/contract/types"
)

type captureRoundTrip func(*http.Request) (*http.Response, error)

func (f captureRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRelaxedBoolOmitempty(t *testing.T) {
	var bodies []string
	client := NewClient("http://example.com", WithHTTPClient(&http.Client{Transport: captureRoundTrip(func(r *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}))
	falseValue := types.RelaxedBool(false)
	if _, err := client.Op(context.Background(), &types.In{Ok: &falseValue}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 1 || !strings.Contains(bodies[0], ` + "`" + `"ok":false` + "`" + `) {
		t.Fatalf("body = %v, want ok:false", bodies)
	}
	if _, err := client.Op(context.Background(), &types.In{}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || strings.Contains(bodies[1], "ok") {
		t.Fatalf("body = %q, want no ok field", bodies[1])
	}
}
`,
		},
		{
			name: "relaxed_union_arm",
			source: relaxedSource(`@relaxedInput`, `union Choice {
    ok: Boolean
    count: Integer
}

structure In {
    choice: Choice
}`, ""),
			run: "^TestRelaxedUnionArm$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type unionService struct{ got types.Choice }

func (s *unionService) Op(ctx context.Context, in *types.In) (*types.Unit, error) {
	s.got = in.Choice
	return &types.Unit{}, nil
}

func TestRelaxedUnionArm(t *testing.T) {
	s := &unionService{}
	h := NewServer(s)
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"choice":{"ok":"true"}}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := s.got.(*types.ChoiceOk); !ok {
		t.Fatalf("decoded union = %T, want *types.ChoiceOk", s.got)
	}
}
`,
		},
		{
			name: "collections_stay_strict",
			source: relaxedSource(`@relaxedInput`, `list Counts {
    member: Integer
}

structure In {
    counts: Counts
}`, ""),
			run: "^TestCollectionsStayStrict$",
			test: `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type listService struct{}

func (s *listService) Op(ctx context.Context, in *types.In) (*types.Unit, error) {
	return &types.Unit{}, nil
}

func TestCollectionsStayStrict(t *testing.T) {
	h := NewServer(&listService{})
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"counts":["1"]}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("quoted element: status = %d, want 400", rec.Code)
	}
	req = httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"counts":[1]}` + "`" + `))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("number element: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
`,
		},
		{
			name: "output_stays_strict",
			source: relaxedSource(`@relaxedInput`, `structure In {
    ok: Boolean
}`, `structure Out {
    ok: Boolean
}`),
			run: "^TestOutputStaysStrict$",
			test: `package contract

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/contract/types"
)

type strictRoundTrip func(*http.Request) (*http.Response, error)

func (f strictRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOutputStaysStrict(t *testing.T) {
	client := NewClient("http://example.com", WithHTTPClient(&http.Client{Transport: strictRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(` + "`" + `{"ok":"true"}` + "`" + `)),
			Request:    r,
		}, nil
	})}))
	if _, err := client.Op(context.Background(), &types.In{}); err == nil {
		t.Fatal("expected strict output decoding to fail")
	}
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := assembleProtocolSources(t, tc.source)
			files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			requireNoGeneratorDanger(t, diags)
			dir := t.TempDir()
			writeFiles(t, dir, files)
			if err := os.WriteFile(filepath.Join(dir, "relaxed_test.go"), []byte(tc.test), 0o644); err != nil {
				t.Fatal(err)
			}
			runGo(t, dir, "test", "-count=1", "-run", tc.run, ".")
		})
	}
}

func TestRelaxedInputMisuseIsDanger(t *testing.T) {
	cases := []struct {
		name   string
		source string
		substr string
	}{
		{
			name: "http_bound_member",
			source: relaxedSource("", `structure In {
    @relaxedInput
    @httpQuery("q")
    count: Integer
}`, ""),
			substr: "HTTP",
		},
		{
			name: "output_member",
			source: relaxedSource("", `structure In {
    ok: Boolean
}`, `structure Out {
    @relaxedInput
    ok: Boolean
}`),
			substr: "operation input",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := assembleProtocolSources(t, tc.source)
			_, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			found := false
			for _, d := range diags {
				if d.Severity == validation.Danger && strings.Contains(strings.ToLower(d.Message), strings.ToLower(tc.substr)) {
					found = true
				}
			}
			if !found {
				t.Fatalf("diagnostics = %v, want DANGER containing %q", diags, tc.substr)
			}
		})
	}
}

func relaxedSource(serviceTraits, in, out string) string {
	use := ""
	if strings.Contains(serviceTraits+in+out, "relaxedInput") {
		use = "use gosmithy.protocols#relaxedInput\n"
	}
	operation := `@http(method: "POST", uri: "/op")
operation Op {
    input: In`
	if out != "" {
		operation += `
    output: Out`
	}
	operation += "\n}"
	source := `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson
` + use + `
@httpJson
` + serviceTraits + `
service S {
    operations: [Op]
}

` + operation + `

` + in
	if out != "" {
		source += "\n\n" + out
	}
	return source + "\n"
}

var _ = codegen.File{}
