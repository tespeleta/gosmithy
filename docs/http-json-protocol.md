# `gosmithy.protocols#httpJson`

Milestone 4 adds a generic HTTP+JSON protocol, its HTTP binding index, and
generated Go clients and servers. The protocol is defined by gosmithy, not by
AWS, and uses only traits from the Smithy prelude plus two serialization traits
(`@jsonName`, `@mediaType`, `@timestampFormat`).

## Enabling the protocol

```sh
go run ./cmd/gosmithy generate \
  --service example.library#Library \
  --generator go \
  --protocol gosmithy.protocols#httpJson \
  --module example.com/library \
  --out ./gen \
  testdata/library.smithy

cd gen && go test ./...
```

The embedded definition is assembled with the user's files, so a service may
also apply the trait explicitly:

```smithy
use gosmithy.protocols#httpJson

@httpJson
service Library { ... }
```

The CLI selection is authoritative. If the service applies a different trait
that is marked `@protocolDefinition`, generation fails with a DANGER
diagnostic.

Without `--protocol`, generation remains model-only: the service interface,
client, server, protocol helpers, and HTTP runtime are not emitted.

## Generated module

```
gen/
  go.mod
  service.go         # Service interface, one method per operation
  client.go          # Client, NewClient, ClientOption
  server.go          # Server, NewServer, ServerOption, http.Handler
  protocol.go        # protocol error envelope, timestamp formatting, URL helpers
  types/             # data types and validators
    types.go
    enums.go
    unions.go
    validate.go
    json.go          # only when custom JSON codecs are needed
  internal/smithy/
    validate.go
    json.go          # timestamps and big decimals
    http.go          # ProtocolError and body helpers (protocol mode only)
```

The generated module has no dependencies outside the Go standard library.

## Generated API

```go
type Service interface {
    GetBook(ctx context.Context, input *types.GetBookInput) (*types.GetBookOutput, error)
}

func NewServer(service Service, opts ...ServerOption) *Server
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)

func NewClient(endpoint string, opts ...ClientOption) *Client
func (c *Client) GetBook(ctx context.Context, input *types.GetBookInput, optFns ...func(*Options)) (*types.GetBookOutput, error)
```

Client options: `WithHTTPClient`, `WithBasePath`, `WithHeader`. Per-call
`optFns` receive a copied `Options` value, so per-call headers and base paths do
not mutate the client.

Server options: `WithMaxBodyBytes` (default 10 MiB), `WithErrorLogger`, and
`WithServerBasePath`. The server is a plain `http.Handler`; wrap it for
middleware.

Modeled errors are returned as their generated Go types. Other error responses
become `*smithy.ProtocolError` with `Code`, `Message`, `StatusCode`, and the raw
`Body`.

## Bindings

| Trait | Behavior |
|---|---|
| `@http(method, uri, code)` | Required on every operation. `code` defaults to 200. |
| `@httpLabel` | One required input member per `{label}` URI segment. Percent-encoded on the client and decoded on the server. |
| `@httpQuery` | Query parameter. Lists become repeated keys; scalars use their string form. |
| `@httpQueryParams` | Map of unbound query parameters. Keys bound with `@httpQuery` are excluded. |
| `@httpHeader` | Request/response header. Lists are comma-separated. |
| `@httpPrefixHeaders` | Map of prefixed request/response headers. |
| `@httpPayload` | Raw request/response body. Blob is raw bytes; string is text; other shapes are JSON. |
| `@httpResponseCode` | Output member that overrides the response status. |
| `@httpError` | Status code for a modeled error. Defaults to 400 for client errors and 500 for server errors. |

Unbound members are serialized into the JSON body. Bound fields are removed
before marshaling and are overwritten from the HTTP message after unmarshaling.
A payload may not be combined with unbound body members.

Unknown JSON fields are ignored on decode for forward compatibility.

## Errors

Success responses use the `@http` code. Errors use the runtime-defined envelope:

```json
{
  "code": "NotFound",
  "message": "resource not found",
  "resource": "book"
}
```

`code` is the error shape name and all error members are top-level fields.
Protocol-level failures are also JSON:

| Situation | Status | Code |
|---|---:|---|
| Unknown route | 404 | `NotFound` |
| Wrong method | 405 + `Allow` | `MethodNotAllowed` |
| Malformed request | 400 | `BadRequest` |
| Input validation failure | 400 | `ValidationException` (with `errors`) |
| Body limit exceeded | 413 | `PayloadTooLarge` |
| Unhandled service error / panic | 500 | `InternalFailure` |

## Value encoding

| Smithy | JSON |
|---|---|
| string, enum | string |
| boolean | bool |
| byte/short/integer/long | number |
| float/double | number; NaN/Inf are rejected |
| bigInteger/bigDecimal | number without float64 precision loss |
| timestamp | string, using the effective `@timestampFormat` |
| blob | base64 string in JSON, raw bytes for a payload |
| document | raw JSON; numbers decode as `json.Number` |
| list/set | array; sparse entries may be `null` |
| map | object |
| structure | object |
| union | one-member object |
| Unit | `{}` |

Timestamp formats (default `date-time`):

- `date-time` — RFC 3339 / RFC 3339Nano;
- `epoch-seconds` — decimal seconds, exact fractional nanoseconds;
- `epoch-milliseconds` — integer milliseconds (gosmithy extension);
- `http-date` — IMF-fixdate.

Documents are decoded with `json.Decoder.UseNumber`, so numbers inside a
document survive as `json.Number` instead of being rounded through `float64`.

`@jsonName` changes JSON object keys. Custom JSON methods are generated only for
shapes that need them: unions, non-default timestamp formats, and big decimals.
Those methods compose with `encoding/json`, so nested custom values work without
a reflective runtime schema.

## Route matching

Literal segments match exactly and labels match one non-empty segment. Trailing
slashes are optional. A route only matches its declared method; a matching path
with the wrong method returns 405 and an `Allow` header. Ambiguous or
overlapping routes of the same method are rejected at generation time.

## Limitations

- No AWS protocols, signing, authentication, retries, endpoints, streaming,
  event streams, compression, or multipart.
- No client input validation. The server validates decoded input.
- Prefix-header suffixes are lowercased because HTTP header names are
  case-insensitive.
- `@httpChecksumRequired` and `@requestCompression` are not implemented.
- Only `application/json` and payload media types are produced; there is no
  content negotiation.
