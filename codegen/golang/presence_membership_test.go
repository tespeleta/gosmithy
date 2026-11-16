package golang

// Contract tests for required scalar presence and enum membership validation.

import "testing"

func TestRequiredAndEnumValidationContracts(t *testing.T) {
	skipIfShort(t)
	t.Parallel()
	source := `$version: "2.0"
namespace tdd

use gosmithy.protocols#httpJson

@httpJson
service S { operations: [Create] }

@http(method: "POST", uri: "/order")
operation Create {
    input: Order
    output: Unit
}

enum Kind {
    PERSONAL = "personal"
    BUSINESS = "business"
}

intEnum Level {
    LOW = 1
    HIGH = 2
}

list KindList {
    member: Kind
}

map KindMap {
    key: String
    value: Kind
}

union Extra {
    note: String
    count: Integer
}

structure Item {
    @required
    id: String

    @required
    price: BigDecimal

    @required
    @length(min: 2)
    sku: String
}

list ItemList {
    member: Item
}

structure Account {
    @required
    name: String

    @required
    active: Boolean

    @required
    kind: Kind

    alias: Kind
    level: Level
    kinds: KindList
    ratings: KindMap
    extra: Extra
}

structure Order {
    @required
    account: Account

    @required
    items: ItemList
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

type orderService struct{}

func (s *orderService) Create(ctx context.Context, in *types.Order) (*types.Unit, error) {
	return &types.Unit{}, nil
}

type validationEnvelope struct {
	Code   string ` + "`" + `json:"code"` + "`" + `
	Errors []struct {
		Path    string ` + "`" + `json:"path"` + "`" + `
		Message string ` + "`" + `json:"message"` + "`" + `
	} ` + "`" + `json:"errors"` + "`" + `
}

func postOrder(t *testing.T, body string) (int, string) {
	t.Helper()
	h := NewServer(&orderService{})
	req := httptest.NewRequest("POST", "/order", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func wantAccepted(t *testing.T, body string) {
	t.Helper()
	status, response := postOrder(t, body)
	if status != 200 {
		t.Fatalf("status = %d, want 200; body = %s", status, response)
	}
}

func wantValidationError(t *testing.T, body, path, message string) {
	t.Helper()
	status, response := postOrder(t, body)
	if status != 400 {
		t.Fatalf("status = %d, want 400; body = %s", status, response)
	}
	var envelope validationEnvelope
	if err := json.Unmarshal([]byte(response), &envelope); err != nil {
		t.Fatalf("body %q: %v", response, err)
	}
	if envelope.Code != "ValidationException" {
		t.Fatalf("code = %q, want ValidationException", envelope.Code)
	}
	if len(envelope.Errors) != 1 {
		t.Fatalf("errors = %+v, want exactly one", envelope.Errors)
	}
	got := envelope.Errors[0]
	if got.Path != path || got.Message != message {
		t.Fatalf("error = %+v, want {%s %s}", got, path, message)
	}
}

const validOrder = ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal"},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `

func TestRequiredPresence(t *testing.T) {
	wantAccepted(t, validOrder)
	wantAccepted(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"business"},"items":[{"id":"","price":100,"sku":"ab"}]}` + "`" + `)
	wantAccepted(t, ` + "`" + `{"account":{"name":"x","active":false,"kind":"personal"},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `)
	wantAccepted(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal","extra":{"note":"hi"}},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `)

	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal"},"items":[{"id":"a1","sku":"ab"}]}` + "`" + `, "items[0].price", "is required")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal"},"items":[{"id":"a1","price":null,"sku":"ab"}]}` + "`" + `, "items[0].price", "is required")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal"},"items":[{"id":"a1","price":100}]}` + "`" + `, "items[0].sku", "is required")
	wantValidationError(t, ` + "`" + `{"account":{"active":true,"kind":"personal"},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account.name", "is required")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal"},"items":[{"price":100,"sku":"ab"}]}` + "`" + `, "items[0].id", "is required")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","kind":"personal"},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account.active", "is required")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account.kind", "is required")
	wantValidationError(t, ` + "`" + `{"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account", "is required")
}

func TestEnumMembership(t *testing.T) {
	wantAccepted(t, validOrder)
	wantAccepted(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal","level":2,"kinds":["personal","business"],"ratings":{"a":"business"}},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `)

	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"alien"},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account.kind", "must be one of: personal, business")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal","alias":"alien"},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account.alias", "must be one of: personal, business")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal","level":3},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account.level", "must be one of: 1, 2")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal","kinds":["personal","alien"]},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account.kinds[1]", "must be one of: personal, business")
	wantValidationError(t, ` + "`" + `{"account":{"name":"x","active":true,"kind":"personal","ratings":{"a":"alien"}},"items":[{"id":"a1","price":100,"sku":"ab"}]}` + "`" + `, "account.ratings[a]", "must be one of: personal, business")
}
`
	runOptionalContract(t, source, testSrc, "^TestRequiredPresence$|^TestEnumMembership$")
}
