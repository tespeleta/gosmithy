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

// TestRelaxedContracts keeps runtime coverage for relaxed decoding, which
// cannot be asserted statically: quoted scalars, byte ranges, payloads, unions,
// nested structures, member overrides, and strict output. Each module packs
// several operations into one service instead of one module per contract.
func TestRelaxedContracts(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	cases := []struct {
		name   string
		source string
		file   string
		test   string
	}{
		{
			name:   "service_relaxed",
			source: relaxedServiceModel,
			file:   "relaxed_service_test.go",
			test:   relaxedServiceTests,
		},
		{
			name:   "strict_member_relaxed",
			source: strictRelaxedModel,
			file:   "strict_relaxed_test.go",
			test:   strictRelaxedTests,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := assembleProtocolSources(t, tc.source)
			files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			requireNoGeneratorDanger(t, diags)
			dir := t.TempDir()
			writeFiles(t, dir, files)
			if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.test), 0o644); err != nil {
				t.Fatal(err)
			}
			runGo(t, dir, "test", ".")
		})
	}
}

const relaxedServiceModel = `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson
use gosmithy.protocols#relaxedInput

@httpJson
@relaxedInput
service S {
    operations: [
        Scalars
        ByteRange
        InvalidBool
        Nested
        PayloadNumber
        UnionArm
        MemberOptOut
        OutputStrict
    ]
}

structure ScalarsIn {
    ok: Boolean
    count: Integer
    ratio: Double
    amount: BigDecimal
}

structure ScalarsOut {
    ok: Boolean
    count: Integer
    ratio: Double
    amount: BigDecimal
}

@http(method: "POST", uri: "/scalars")
operation Scalars {
    input: ScalarsIn
    output: ScalarsOut
}

structure ByteRangeIn {
    count: Byte
}

structure ByteRangeOut {
    count: Byte
}

@http(method: "POST", uri: "/byte-range")
operation ByteRange {
    input: ByteRangeIn
    output: ByteRangeOut
}

structure InvalidBoolIn {
    ok: Boolean
}

@http(method: "POST", uri: "/invalid-bool")
operation InvalidBool {
    input: InvalidBoolIn
}

structure Inner {
    ok: Boolean
    count: Integer
}

structure NestedIn {
    inner: Inner
}

@http(method: "POST", uri: "/nested")
operation Nested {
    input: NestedIn
}

structure PayloadNumberIn {
    @httpPayload
    count: Integer
}

@http(method: "POST", uri: "/payload-number")
operation PayloadNumber {
    input: PayloadNumberIn
}

union Choice {
    ok: Boolean
    count: Integer
}

structure UnionArmIn {
    choice: Choice
}

@http(method: "POST", uri: "/union-arm")
operation UnionArm {
    input: UnionArmIn
}

structure MemberOptOutIn {
    ok: Boolean

    @relaxedInput(enabled: false)
    count: Integer
}

structure MemberOptOutOut {
    ok: Boolean
    count: Integer
}

@http(method: "POST", uri: "/member-opt-out")
operation MemberOptOut {
    input: MemberOptOutIn
    output: MemberOptOutOut
}

structure OutputStrictIn {
    ok: Boolean
}

structure OutputStrictOut {
    ok: Boolean
}

@http(method: "POST", uri: "/output-strict")
operation OutputStrict {
    input: OutputStrictIn
    output: OutputStrictOut
}
`

const relaxedServiceTests = `package contract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type relaxedRoundTrip func(*http.Request) (*http.Response, error)

func (f relaxedRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func relaxedBoolValue(v *types.RelaxedBool) *bool {
	if v == nil {
		return nil
	}
	b := v.Value()
	return &b
}

type relaxedService struct {
	nested  *types.Inner
	union   types.Choice
	payload *int32
}

func (s *relaxedService) Scalars(ctx context.Context, in *types.ScalarsIn) (*types.ScalarsOut, error) {
	return &types.ScalarsOut{Ok: relaxedBoolValue(in.Ok), Count: in.Count, Ratio: in.Ratio, Amount: in.Amount}, nil
}

func (s *relaxedService) ByteRange(ctx context.Context, in *types.ByteRangeIn) (*types.ByteRangeOut, error) {
	return &types.ByteRangeOut{Count: in.Count}, nil
}

func (s *relaxedService) InvalidBool(ctx context.Context, in *types.InvalidBoolIn) (*types.Unit, error) {
	return &types.Unit{}, nil
}

func (s *relaxedService) Nested(ctx context.Context, in *types.NestedIn) (*types.Unit, error) {
	s.nested = in.Inner
	return &types.Unit{}, nil
}

func (s *relaxedService) PayloadNumber(ctx context.Context, in *types.PayloadNumberIn) (*types.Unit, error) {
	s.payload = in.Count
	return &types.Unit{}, nil
}

func (s *relaxedService) UnionArm(ctx context.Context, in *types.UnionArmIn) (*types.Unit, error) {
	s.union = in.Choice
	return &types.Unit{}, nil
}

func (s *relaxedService) MemberOptOut(ctx context.Context, in *types.MemberOptOutIn) (*types.MemberOptOutOut, error) {
	return &types.MemberOptOutOut{Ok: relaxedBoolValue(in.Ok), Count: in.Count}, nil
}

func (s *relaxedService) OutputStrict(ctx context.Context, in *types.OutputStrictIn) (*types.OutputStrictOut, error) {
	return &types.OutputStrictOut{Ok: relaxedBoolValue(in.Ok)}, nil
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServiceRelaxed(t *testing.T) {
	h := NewServer(&relaxedService{})
	rec := post(h, "/scalars", ` + "`" + `{"ok":"true","count":"100","ratio":"1.5","amount":"100.50"}` + "`" + `)
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

func TestByteRange(t *testing.T) {
	h := NewServer(&relaxedService{})
	for _, body := range []string{` + "`" + `{"count":"300"}` + "`" + `, ` + "`" + `{"count":"abc"}` + "`" + `} {
		if rec := post(h, "/byte-range", body); rec.Code != 400 {
			t.Fatalf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
	if rec := post(h, "/byte-range", ` + "`" + `{"count":"127"}` + "`" + `); rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestInvalidBool(t *testing.T) {
	h := NewServer(&relaxedService{})
	if rec := post(h, "/invalid-bool", ` + "`" + `{"ok":"yes"}` + "`" + `); rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNestedInput(t *testing.T) {
	s := &relaxedService{}
	h := NewServer(s)
	rec := post(h, "/nested", ` + "`" + `{"inner":{"ok":"true","count":"100"}}` + "`" + `)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if s.nested == nil || s.nested.Ok == nil || !bool(*s.nested.Ok) || s.nested.Count == nil || *s.nested.Count != 100 {
		t.Fatalf("decoded = %+v", s.nested)
	}
}

func TestPayloadNumber(t *testing.T) {
	s := &relaxedService{}
	h := NewServer(s)
	for _, body := range []string{` + "`" + `100` + "`" + `, ` + "`" + `"100"` + "`" + `} {
		if rec := post(h, "/payload-number", body); rec.Code != 200 {
			t.Fatalf("body %s: status = %d, body = %s", body, rec.Code, rec.Body.String())
		}
	}
	if s.payload == nil || *s.payload != 100 {
		t.Fatalf("decoded payload = %v", s.payload)
	}
	if rec := post(h, "/payload-number", ` + "`" + `"abc"` + "`" + `); rec.Code != 400 {
		t.Fatalf("invalid payload: status = %d, want 400", rec.Code)
	}
}

func TestRelaxedUnionArm(t *testing.T) {
	s := &relaxedService{}
	h := NewServer(s)
	rec := post(h, "/union-arm", ` + "`" + `{"choice":{"ok":"true"}}` + "`" + `)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := s.union.(*types.ChoiceOk); !ok {
		t.Fatalf("decoded union = %T, want *types.ChoiceOk", s.union)
	}
}

func TestMemberOptOut(t *testing.T) {
	h := NewServer(&relaxedService{})
	if rec := post(h, "/member-opt-out", ` + "`" + `{"ok":"true","count":"100"}` + "`" + `); rec.Code != 400 {
		t.Fatalf("strict count: status = %d, want 400", rec.Code)
	}
	if rec := post(h, "/member-opt-out", ` + "`" + `{"ok":"true","count":100}` + "`" + `); rec.Code != 200 {
		t.Fatalf("relaxed bool: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestOutputStaysStrict(t *testing.T) {
	client := NewClient("http://example.com", WithHTTPClient(&http.Client{Transport: relaxedRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(` + "`" + `{"ok":"true"}` + "`" + `)),
			Request:    r,
		}, nil
	})}))
	if _, err := client.OutputStrict(context.Background(), &types.OutputStrictIn{}); err == nil {
		t.Fatal("expected strict output decoding to fail")
	}
}
`

const strictRelaxedModel = `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson
use gosmithy.protocols#relaxedInput

@httpJson
service S {
    operations: [
        Strict
        MemberOptIn
    ]
}

structure StrictIn {
    ok: Boolean
    count: Integer
    amount: BigDecimal
}

structure StrictOut {
    ok: Boolean
    count: Integer
    amount: BigDecimal
}

@http(method: "POST", uri: "/strict")
operation Strict {
    input: StrictIn
    output: StrictOut
}

structure MemberOptInIn {
    @relaxedInput
    ok: Boolean
    count: Integer
}

structure MemberOptInOut {
    ok: Boolean
    count: Integer
}

@http(method: "POST", uri: "/member-opt-in")
operation MemberOptIn {
    input: MemberOptInIn
    output: MemberOptInOut
}
`

const strictRelaxedTests = `package contract

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type strictService struct{}

func (s *strictService) Strict(ctx context.Context, in *types.StrictIn) (*types.StrictOut, error) {
	return &types.StrictOut{Ok: in.Ok, Count: in.Count, Amount: in.Amount}, nil
}

func relaxedBoolValue(v *types.RelaxedBool) *bool {
	if v == nil {
		return nil
	}
	b := v.Value()
	return &b
}

func (s *strictService) MemberOptIn(ctx context.Context, in *types.MemberOptInIn) (*types.MemberOptInOut, error) {
	return &types.MemberOptInOut{Ok: relaxedBoolValue(in.Ok), Count: in.Count}, nil
}

func TestStrictRejectsQuoted(t *testing.T) {
	h := NewServer(&strictService{})
	for _, body := range []string{
		` + "`" + `{"ok":"true"}` + "`" + `,
		` + "`" + `{"count":"100"}` + "`" + `,
		` + "`" + `{"amount":"100.50"}` + "`" + `,
	} {
		req := httptest.NewRequest("POST", "/strict", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Fatalf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
	req := httptest.NewRequest("POST", "/strict", strings.NewReader(` + "`" + `{"ok":true,"count":100,"amount":100.50}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("strict number: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestMemberOptIn(t *testing.T) {
	h := NewServer(&strictService{})
	req := httptest.NewRequest("POST", "/member-opt-in", strings.NewReader(` + "`" + `{"ok":true,"count":"100"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("strict count: status = %d, want 400", rec.Code)
	}
	req = httptest.NewRequest("POST", "/member-opt-in", strings.NewReader(` + "`" + `{"ok":"true","count":100}` + "`" + `))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("relaxed bool: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
`

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

func generatedFileContent(t *testing.T, files []codegen.File, path string) string {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			return string(file.Content)
		}
	}
	t.Fatalf("generated file %s not found", path)
	return ""
}

// TestRelaxedGeneratedShapes covers two properties that used to need their own
// generated modules: pointer RelaxedBool tags and strict collection elements.
func TestRelaxedGeneratedShapes(t *testing.T) {
	t.Run("pointer_bool_omitempty", func(t *testing.T) {
		m := assembleProtocolSources(t, `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson
use gosmithy.protocols#relaxedInput

@httpJson
@relaxedInput
service S { operations: [Op] }

@http(method: "POST", uri: "/op")
operation Op { input: In }

structure In {
    ok: Boolean
}
`)
		files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		requireNoGeneratorDanger(t, diags)
		assertGeneratedContent(t, files, []shapeAssertion{
			has("types/types.go", "Ok *RelaxedBool `json:\"ok,omitempty\"`"),
			has("types/relaxed.go",
				"func (b RelaxedBool) MarshalJSON()",
				`[]byte("false")`),
		})
	})

	t.Run("collections_stay_strict", func(t *testing.T) {
		m := assembleProtocolSources(t, `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson
use gosmithy.protocols#relaxedInput

@httpJson
@relaxedInput
service S { operations: [Op] }

@http(method: "POST", uri: "/op")
operation Op { input: In }

list Counts {
    member: Integer
}

structure In {
    counts: Counts
}
`)
		files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		requireNoGeneratorDanger(t, diags)
		assertGeneratedContent(t, files, []shapeAssertion{
			has("types/types.go", "type Counts []int32"),
			lacks("types/types.go", "Optional[int32]", "RelaxedInt"),
		})
	})
}
