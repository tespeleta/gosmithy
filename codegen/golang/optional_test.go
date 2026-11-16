package golang

import (
	"context"
	"fmt"
	"go/version"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
)

func skipIfNoOptionalToolchain(t *testing.T) {
	t.Helper()
	if version.Compare(version.Lang(runtime.Version()), "go1.24") < 0 {
		t.Skip("Optional[T] requires Go 1.24 or newer")
	}
}

func runOptionalProtocolGenerator(t *testing.T, m *model.Model, service model.ShapeID, module string) ([]codegen.File, []codegen.Diagnostic, error) {
	t.Helper()
	req := &codegen.Request{
		Model:    m,
		Service:  service,
		Version:  model.V2_0,
		Protocol: &codegen.Protocol{ID: httpjson.ID},
		Target:   codegen.Target{Name: GeneratorName, Generators: []string{GeneratorName}},
		Options:  map[string]string{"module": module, "useOptionals": "true", "goMod": "true"},
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

func runOptionalContract(t *testing.T, source, testSrc, run string) {
	t.Helper()
	m := assembleProtocolSources(t, source)
	files, diags, err := runOptionalProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	requireNoGeneratorDanger(t, diags)
	dir := t.TempDir()
	writeFiles(t, dir, files)
	if err := os.WriteFile(filepath.Join(dir, "optional_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	runGo(t, dir, "test", "-run", run, ".")
}

func TestOptionalContracts(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	t.Run("scalars", func(t *testing.T) {
		t.Parallel()
		source := `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson

@httpJson
service S { operations: [Op] }

@http(method: "POST", uri: "/op")
operation Op {
    input: In
    output: Out
}

@enum([
    { name: "RED", value: "red" },
    { name: "BLUE", value: "blue" }
])
string Color

structure In {
    name: String
    ok: Boolean
    count: Integer
    ratio: Double
    amount: BigDecimal
    when: Timestamp
    color: Color

    @required
    id: String
}

structure Out {
    name: String
    ok: Boolean
    count: Integer
    ratio: Double
    amount: BigDecimal
    when: Timestamp
    color: Color

    @required
    id: String
}
`
		testSrc := `package contract

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type scalarService struct{}

func (s *scalarService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	return &types.Out{Name: in.Name, Ok: in.Ok, Count: in.Count, Ratio: in.Ratio, Amount: in.Amount, When: in.When, Color: in.Color, Id: in.Id}, nil
}

func TestOptionalScalars(t *testing.T) {
	h := NewServer(&scalarService{})
	body := ` + "`" + `{"name":"x","ok":false,"count":0,"ratio":1.5,"amount":100.50,"when":"2024-01-02T03:04:05Z","color":"red","id":"i"}` + "`" + `
	req := httptest.NewRequest("POST", "/op", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if string(out["name"]) != ` + "`" + `"x"` + "`" + ` || string(out["ok"]) != "false" || string(out["count"]) != "0" || string(out["ratio"]) != "1.5" || string(out["amount"]) != "100.50" || string(out["color"]) != ` + "`" + `"red"` + "`" + ` || string(out["id"]) != ` + "`" + `"i"` + "`" + ` {
		t.Fatalf("body = %s", rec.Body.String())
	}

	req = httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"id":"i"}` + "`" + `))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != ` + "`" + `{"id":"i"}` + "`" + ` {
		t.Fatalf("absent body = %s status = %d", rec.Body.String(), rec.Code)
	}

	req = httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"name":null,"id":"i"}` + "`" + `))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), ` + "`" + `"name":null` + "`" + `) {
		t.Fatalf("null body = %s status = %d", rec.Body.String(), rec.Code)
	}
}
`
		runOptionalContract(t, source, testSrc, "^TestOptionalScalars$")
	})

	t.Run("relaxed", func(t *testing.T) {
		t.Parallel()
		source := `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson
use gosmithy.protocols#relaxedInput

@httpJson
@relaxedInput
service S { operations: [Op] }

@http(method: "POST", uri: "/op")
operation Op {
    input: In
    output: Out
}

structure In {
    ok: Boolean
    count: Integer
    amount: BigDecimal
}

structure Out {
    ok: Boolean
    count: Integer
    amount: BigDecimal
}
`
		testSrc := `package contract

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/contract/types"
)

type relaxedService struct{}

func boolOptional(v types.Optional[types.RelaxedBool]) types.Optional[bool] {
	if v.IsNull() {
		return types.NewOptionalNull[bool]()
	}
	if !v.HasValue() {
		return types.Optional[bool]{}
	}
	return types.NewOptional(v.MustValue().Value())
}

func (s *relaxedService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	return &types.Out{Ok: boolOptional(in.Ok), Count: in.Count, Amount: in.Amount}, nil
}

func TestOptionalRelaxed(t *testing.T) {
	h := NewServer(&relaxedService{})
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"ok":"true","count":"100","amount":"100.50"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if string(out["ok"]) != "true" || string(out["count"]) != "100" || string(out["amount"]) != "100.50" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
`
		runOptionalContract(t, source, testSrc, "^TestOptionalRelaxed$")
	})

	t.Run("formatted_timestamp", func(t *testing.T) {
		t.Parallel()
		source := `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson

@httpJson
service S { operations: [Op] }

@http(method: "POST", uri: "/op")
operation Op {
    input: In
    output: Out
}

structure In {
    @timestampFormat("epoch-seconds")
    when: Timestamp
}

structure Out {
    @timestampFormat("epoch-seconds")
    when: Timestamp
}
`
		testSrc := `package contract

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/contract/types"
)

type timestampService struct{}

func (s *timestampService) Op(ctx context.Context, in *types.In) (*types.Out, error) {
	return &types.Out{When: in.When}, nil
}

func TestOptionalFormattedTimestamp(t *testing.T) {
	h := NewServer(&timestampService{})
	req := httptest.NewRequest("POST", "/op", strings.NewReader(` + "`" + `{"when":"1706933106"}` + "`" + `))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if string(out["when"]) != ` + "`" + `"1706933106"` + "`" + ` {
		t.Fatalf("body = %s", rec.Body.String())
	}
	_ = time.Second
}
`
		runOptionalContract(t, source, testSrc, "^TestOptionalFormattedTimestamp$")
	})
}

func TestOptionalValidation(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	m := assembleProtocolSources(t, `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson

@httpJson
service S { operations: [Op] }

@http(method: "POST", uri: "/op")
operation Op { input: In }

structure In {
    @length(min: 1, max: 5)
    name: String

    @range(min: 0, max: 10)
    count: Integer

    @range(min: -100000000000000000000000000000, max: 100000000000000000000000000000)
    big: BigInteger

    @range(min: 0.000000000000000000000000000001, max: 100.5)
    dec: BigDecimal

    @required
    @length(min: 1, max: 5)
    id: String
}
`)
	files, diags, err := runOptionalProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	requireNoGeneratorDanger(t, diags)
	dir := t.TempDir()
	writeFiles(t, dir, files)
	testSrc := `package types

import (
	"math/big"
	"testing"
)

func TestOptionalValidationRules(t *testing.T) {
	ok := In{Id: "i"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("absent optional: %v", err)
	}
	badName := In{Id: "i", Name: NewOptional("toolong")}
	if err := badName.Validate(); err == nil {
		t.Fatal("want name length error")
	}
	nullName := In{Id: "i", Name: NewOptionalNull[string]()}
	if err := nullName.Validate(); err != nil {
		t.Fatalf("null optional: %v", err)
	}
	badCount := In{Id: "i", Count: NewOptional(int32(11))}
	if err := badCount.Validate(); err == nil {
		t.Fatal("want count range error")
	}
	huge, _ := new(big.Int).SetString("100000000000000000000000000001", 10)
	badBig := In{Id: "i", Big: NewOptional(huge)}
	if err := badBig.Validate(); err == nil {
		t.Fatal("want bigInteger range error")
	}
	nullBig := In{Id: "i", Big: NewOptional[*big.Int](nil)}
	if err := nullBig.Validate(); err != nil {
		t.Fatalf("nil bigInteger: %v", err)
	}
	badDec := In{Id: "i", Dec: NewOptional(MustDecimal("0"))}
	if err := badDec.Validate(); err == nil {
		t.Fatal("want bigDecimal range error")
	}
	invalidDec := In{Id: "i", Dec: NewOptional(Decimal("bad"))}
	if err := invalidDec.Validate(); err != nil {
		t.Fatalf("invalid decimal: %v", err)
	}
	emptyId := In{}
	if err := emptyId.Validate(); err == nil {
		t.Fatal("want required id length error")
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "types", "optional_validate_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	runGo(t, dir, "test", "-run", "TestOptionalValidationRules", "./types")
}

func TestUseOptionalsFalse(t *testing.T) {
	m := assembleProtocolSources(t, `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson

@httpJson
service S { operations: [Op] }

@http(method: "POST", uri: "/op")
operation Op { input: In }

structure In {
    name: String
    @required
    id: String
}
`)
	files, diags, err := runProtocolGenerator(t, m, model.MustShapeID("tdd#S"), "example.com/contract")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	requireNoGeneratorDanger(t, diags)
	found := false
	for _, file := range files {
		switch file.Path {
		case "go.mod":
			if !strings.Contains(string(file.Content), "go 1.22") {
				t.Errorf("go.mod = %s", file.Content)
			}
		case "types/types.go":
			if strings.Contains(string(file.Content), "Optional[") {
				t.Errorf("types.go still uses Optional:\n%s", file.Content)
			}
			if strings.Contains(string(file.Content), "Name *string") {
				found = true
			}
		case "types/optional.go":
			t.Error("types/optional.go should not be generated")
		}
	}
	if !found {
		t.Error("want pointer fields for useOptionals=false")
	}
}
