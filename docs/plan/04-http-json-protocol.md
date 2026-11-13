# Milestone 4 — Generic HTTP+JSON protocol, client, and server

Status: planned · Estimate: **9 dev-days** · Depends on: Milestone 3
Deliverable: generated Go clients and servers for the project's own generic
HTTP+JSON protocol.

---

## 1. Unit of work

Add the first wire protocol and generate both sides of it. Client, server,
HTTP bindings, serialization, and the protocol definition are one unit: neither
side is useful alone, and the round-trip test is what proves the protocol.

This milestone completes the original goal — server, models, clients,
validators — **without any AWS-specific behavior**.

**Demo**

```sh
$ go run ./cmd/gosmithy generate \
    --service example.library#Library \
    --generator go \
    --protocol gosmithy.protocols#httpJson \
    --module example.com/library \
    --out ./gen \
    testdata/library.smithy

$ cd gen && go test ./...
```

The generated module contains:

```
gen/
  go.mod
  types/...            # from Milestone 3
  internal/smithy/...  # types + validation + http/json codecs
  service.go           # Service interface
  server.go            # Handler / NewServer
  client.go            # Client / NewClient
  protocol.go          # httpJson codec entry points
```

---

## 2. Scope

### In

- Protocol definition `gosmithy.protocols#httpJson` (embedded model).
- HTTP binding index for core Smithy traits:
  - `@http(method, uri, code)`;
  - `@httpLabel`;
  - `@httpQuery`, `@httpQueryParams`;
  - `@httpHeader`, `@httpPrefixHeaders`;
  - `@httpPayload`;
  - `@httpResponseCode`;
  - `@httpError`.
- Serialization traits:
  - `@jsonName`;
  - `@timestampFormat` (default date-time; support the four Smithy formats);
  - `@mediaType` (for payloads).
- Protocol semantics (see §3).
- Runtime helpers in `internal/smithy`:
  - JSON encoding/decoding;
  - timestamp formats;
  - blob base64;
  - document passthrough;
  - protocol error type;
  - validation error mapping.
- Generated client:
  - operation methods;
  - URI construction with labels/query;
  - headers;
  - JSON/payload bodies;
  - response status/header/body decoding;
  - modeled error decoding + unknown protocol error;
  - `context.Context`, options (`WithHTTPClient`, `WithBasePath`).
- Generated server:
  - `Service` interface;
  - `http.Handler` with routing (method + URI pattern);
  - label/query/header/body decoding;
  - input validation → 400;
  - status/header/body encoding;
  - modeled error encoding;
  - unknown route/malformed request handling.
- Tests:
  - `httptest` round-trip for each binding type;
  - request snapshots (method, path, query, headers, body);
  - response/error decoding;
  - validation failure;
  - golden generated code + compile.

### Out

- AWS protocols (`awsJson1_1`, `restJson1`, `awsQuery`, `restXml`, ...), AWS
  auth, SigV4, endpoints.
- Authentication/authorization, signing.
- Streaming payloads, event streams, multipart, compression.
- Retries, timeouts policy, middleware framework, pagination helpers.
- Server-side concurrency limits, graceful shutdown opinions.
- HTTP/2-specific behavior.
- `@httpChecksumRequired`, `@requestCompression`.
- Full content negotiation; only `application/json` and raw payload media types.

---

## 3. Protocol: `gosmithy.protocols#httpJson`

### 3.1 Trait definition

```smithy
$version: "2.0"
namespace gosmithy.protocols

@protocolDefinition(
    traits: [
        http
        httpError
        httpHeader
        httpLabel
        httpPayload
        httpPrefixHeaders
        httpQuery
        httpQueryParams
        httpResponseCode
        jsonName
        mediaType
        timestampFormat
    ]
)
@trait(selector: "service")
structure httpJson {}
```

The generator loads this model automatically when
`--protocol gosmithy.protocols#httpJson` is selected. A service may apply the
trait explicitly; the CLI selection is authoritative in MVP. If the service has
a different protocol trait, generation fails with a clear diagnostic.

### 3.2 Request

- Method and URI from `@http`. Missing `@http` is a DANGER.
- URI labels are `{name}` segments; one path segment each; percent-encoded on
  the client and decoded on the server. A label must bind a required input
  member with `@httpLabel`.
- Query parameters come from `@httpQuery` members and `@httpQueryParams` map.
  Lists are repeated keys. Booleans/numbers/timestamps use their string form.
- Headers come from `@httpHeader` members and `@httpPrefixHeaders` maps.
  Lists are comma-separated on one header.
- `@httpPayload` binds exactly one input member to the raw body:
  - blob/string → raw bytes/text;
  - structure/document → JSON;
  - other shapes → JSON.
- Remaining input members form a JSON object body.
- If there is no payload and no body member, send no body and no
  `Content-Type`.
- If there is a body, `Content-Type: application/json` (or the payload's
  media type).
- `Content-Length` is set by `net/http`.

### 3.3 Response

- Success status from `@http(code)`, default 200.
- `@httpResponseCode` member overrides it.
- Output headers from `@httpHeader` / `@httpPrefixHeaders`.
- `@httpPayload` output member → raw body.
- Remaining output members → JSON body.
- No body members and no payload → no body, no `Content-Type`.
- Response `Content-Type` is `application/json` when a JSON body exists.

### 3.4 Errors

- `@error("client"|"server")` determines fault; `@httpError(code)` overrides
  the status; default 400 for client, 500 for server.
- Error response body:

```json
{
  "code": "NotFound",
  "message": "resource not found",
  "resource": "book"
}
```

- `code` is the error shape name; `message` is the `Message`/`message` member
  if present; all other error structure members are top-level fields.
- `Content-Type: application/json`.
- Known errors are decoded by `code` into their generated Go types.
- Unknown codes produce `*smithy.ProtocolError` with code, message, status, and
  raw body.
- Protocol-level failures (unroutable request, malformed JSON, validation) use:
  - 404 `NotFound` for unknown route;
  - 400 `BadRequest` for malformed request;
  - 400 `ValidationException` with validation messages;
  - 500 `InternalFailure` for unhandled service errors.

These protocol errors are not modeled shapes; they are defined in the runtime.

### 3.5 Value encoding

| Smithy | Encoding |
|---|---|
| string | JSON string |
| boolean | JSON bool |
| byte/short/integer/long | JSON number (int) |
| float/double | JSON number; reject NaN/Inf |
| bigInteger/bigDecimal | JSON number (string form); no precision loss |
| timestamp | RFC 3339 string by default; `@timestampFormat` overrides |
| blob | base64 string in JSON; raw bytes for payload |
| document | raw JSON |
| list/set | JSON array; sparse allows `null` |
| map | JSON object |
| structure | JSON object |
| union | JSON object with exactly one member key |
| enum | JSON string |
| intEnum | JSON number |
| Unit | `{}` |

- `@jsonName` changes the JSON field name.
- Absent optional members are omitted (`omitempty` / explicit presence).
- Sparse collections may contain `null`.
- Trailing/unknown JSON fields are ignored by default; document as forward
  compatibility.

### 3.6 URI matching

- Literal segments match exactly; labels match one non-empty segment.
- Trailing slash is optional on match.
- Duplicate or overlapping routes are a DANGER at generation time.
- Percent-encoding uses `url.PathEscape` per label (with `/` escaped) and
  `url.QueryEscape` for query values.
- A route only matches its declared HTTP method; wrong method → 405 with
  `Allow` header.

---

## 4. Runtime design (`internal/smithy`)

Keep the runtime small and boring:

```go
package smithy

type ProtocolError struct {
    Code       string
    Message    string
    StatusCode int
    Body       []byte
}
func (e *ProtocolError) Error() string

type ValidationError struct{ Path, Message string }
type ValidationErrors []ValidationError
// Add, AddNested, Err — from Milestone 3
```

Serialization approach:

- Use `encoding/json` for structures/lists/maps with generated tags.
- Generate custom `MarshalJSON`/`UnmarshalJSON` for:
  - unions (one-of object);
  - shapes needing non-default timestamp formats;
  - big numbers (string/number precision).
- Generate per-protocol encode/decode helpers only where `encoding/json` cannot
  express the semantics.
- Keep protocol code (URI building, headers, status) in generated code, not in
  reflection-based runtime helpers. Generated code is easier to debug and keeps
  the runtime dependency-free.

Timestamp formats:

- `date-time` → RFC 3339 (`2006-01-02T15:04:05Z07:00`);
- `epoch-seconds` → decimal seconds, fractional allowed;
- `epoch-milliseconds` → integer milliseconds;
- `http-date` → `http.TimeFormat`.

Default for `httpJson` is `date-time`, unlike AWS JSON's epoch-seconds. This is
our protocol, so the default is documented and explicit.

Body limits:

- Server option `WithMaxBodyBytes` (default e.g. 10 MiB) for JSON/payload
  requests; exceed → 413 `PayloadTooLarge`.
- Client does not impose a default limit; caller's `http.Client`/`Transport`
  controls it.

---

## 5. Generated API

### 5.1 Types

Unchanged from Milestone 3, plus protocol-aware JSON tags and custom codecs where
needed. Types stay protocol-neutral; the `httpJson` codec lives in the root
package or `internal/smithy`, not in `types`.

### 5.2 Service interface

```go
type Service interface {
    GetCity(ctx context.Context, input *types.GetCityInput) (*types.GetCityOutput, error)
    ...
}
```

- One method per operation in the service closure.
- `Unit` input/output still uses `*types.Unit` (or `struct{}` alias) for a
  uniform signature.

### 5.3 Server

```go
type Server struct{ ... }

func NewServer(svc Service, opts ...ServerOption) *Server
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

- Options: `WithMaxBodyBytes`, `WithErrorLogger` (optional), `WithBasePath`.
- Decode → validate input → call service → encode output/error.
- Panic recovery → 500 `InternalFailure` (log optional).
- No middleware framework; users wrap the handler.

### 5.4 Client

```go
type Client struct{ ... }

func NewClient(endpoint string, opts ...ClientOption) *Client
func (c *Client) GetCity(ctx context.Context, input *types.GetCityInput, optFns ...func(*Options)) (*types.GetCityOutput, error)
```

- Options: `WithHTTPClient`, `WithBasePath`, `WithHeader` (static headers).
- Build request, execute, decode response/error.
- No retries/signing.
- Return `*types.<Error>` for modeled errors, `*smithy.ProtocolError`
  otherwise.

### 5.5 Validation

- Server validates operation input after decode: `if err := input.Validate(); err
  != nil { return 400 ValidationException }`.
- Validation errors include path + message; protocol error body carries a list:
  `{"code":"ValidationException","message":"...","errors":[{"path":"...","message":"..."}]}`.
- Client does not validate inputs by default; a `WithValidateInputs` option can
  be added later.

---

## 6. Work breakdown

| Unit | Work | Estimate |
|---|---|---|
| U1 | Protocol trait model, protocol registry/selection, embedded model loading | 0.5d |
| U2 | HTTP binding index: parse/validate `@http*` traits, route table, overlaps | 1.5d |
| U3 | Runtime JSON/timestamp/blob/document/error helpers | 1.5d |
| U4 | Generated client: request building, label/query/header/payload, response decode | 1.5d |
| U5 | Generated server: router, decode, validation, encode, errors | 1.5d |
| U6 | Custom codecs for unions / timestamps / big numbers as needed | 0.75d |
| U7 | Round-trip + snapshot + error tests; golden/compile tests | 1.25d |
| U8 | Docs: protocol spec, limitations, examples | 0.5d |

### U2 details

Create `model/httpbinding` (or `codegen/golang/httpbinding`, but it is generic
model knowledge and belongs in `model`):

```go
type Binding struct {
    Operation   ShapeID
    Method      string
    URI         string
    Labels      map[string]ShapeID
    Query       map[string]ShapeID
    QueryParams ShapeID
    Headers     map[string]ShapeID
    PrefixHeaders ShapeID
    Payload     ShapeID
    ResponseCode ShapeID
}
```

Validate:

- `@http` exists and method/uri are valid;
- every URI label has exactly one `@httpLabel` member with a matching name;
- labels are required, scalar/string-like targets;
- no duplicate bindings (a member bound to more than one location);
- `@httpPayload` at most one;
- response code is an integer shape;
- header/query target types are serializable;
- routes do not overlap ambiguously.

Do not implement a general URI parser library; a small trie/segment matcher is
enough.

### U3 details

- `json.Decoder.UseNumber()` for documents/big numbers.
- Helpers for `time.Time` formatting/parsing per format.
- Base64 via `encoding/base64` (stdlib JSON already does this for `[]byte`).
- Raw body read/write with limits.
- Error envelope encode/decode.

### U4/U5 details

Generate explicit code per operation, not a reflection-based generic dispatcher.
A little repetition is fine; debuggability matters more.

- Client request builder:
  - start with base URL + operation URI;
  - substitute labels with escaped values;
  - append query params;
  - set headers;
  - encode payload or JSON body;
  - set `Content-Type`.
- Server:
  - route table built at construction;
  - match method/path;
  - extract labels/query/headers;
  - read/limit body;
  - decode payload/JSON;
  - validate;
  - call;
  - encode.

---

## 7. Acceptance criteria

- [ ] `go build ./...`, `go test ./...`, `go vet ./...` pass.
- [ ] `gosmithy generate --protocol gosmithy.protocols#httpJson` produces a
      compilable module with server and client.
- [ ] Round-trip passes for:
      path labels, query params, headers, prefix headers, JSON input/output,
      blob payload, modeled client error, modeled server error, unknown error,
      validation failure, empty input/output.
- [ ] Request snapshots assert method, path, query, selected headers, and body.
- [ ] URI routes reject overlapping patterns at generation time.
- [ ] Wrong method → 405; unknown route → 404; malformed body → 400.
- [ ] Timestamp formats round-trip for all four formats.
- [ ] Generated code has no external dependencies.
- [ ] Generated code is deterministic and golden-tested.
- [ ] No AWS-specific trait, header, protocol, or code path exists.

**Demo**

```sh
go run ./cmd/gosmithy generate \
  --service example.library#Library \
  --generator go \
  --protocol gosmithy.protocols#httpJson \
  --module example.com/library \
  --out /tmp/library-gen \
  testdata/library.smithy

(cd /tmp/library-gen && go test ./...)
```

---

## 8. Risks

| Risk | Mitigation |
|---|---|
| Protocol spec grows into a second product | Freeze §3 semantics for M4; streaming/auth/compression are explicitly out. |
| HTTP binding edge cases | Follow `http-bindings.rst` for labels/query/header/payload/status; protocol tests from scratch. |
| Serialization correctness (unions, timestamps) | Generate explicit codecs; table-driven round-trip tests for every shape type. |
| Route matching bugs | Small segment matcher + overlap validation; unit tests per pattern. |
| Error mapping ambiguity | One envelope (`code`, `message`, members); unknown errors become `ProtocolError`. |
| Client/server API churn | Keep options small; no middleware framework; add only when a real need appears. |
| Scope creep toward AWS compatibility | The protocol is `gosmithy.protocols#httpJson`; AWS is a separate future plugin. |

---

## 9. Deliverables

- `gosmithy.protocols#httpJson` definition and documentation.
- HTTP binding index + validation.
- Generated client and server.
- Runtime codecs and protocol errors.
- Round-trip/snapshot/golden tests.
- The complete original goal: parser, model, validators, client, server — all
  generic Smithy, no AWS.
