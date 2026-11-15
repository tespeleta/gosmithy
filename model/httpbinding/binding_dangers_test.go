package httpbinding

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/model"
)

const bindingTestService = "tdd#S"

var bindingDangerCases = []struct {
	name    string
	sources []string
	substr  string
}{
	// D1: sparse collections are valid Smithy but the httpJson protocol has no
	// unambiguous null representation in query strings or headers.
	{
		name: "sparse_list_query",
		sources: []string{bindingModel(`@sparse
list Tags {
    member: String
}

structure In {
    @httpQuery("tag")
    tags: Tags
}`, bindingOpInput)},
		substr: "sparse",
	},
	{
		name: "sparse_list_header",
		sources: []string{bindingModel(`@sparse
list Tags {
    member: String
}

structure In {
    @httpHeader("X-Tag")
    tags: Tags
}`, bindingOpInput)},
		substr: "sparse",
	},
	{
		name: "sparse_map_queryparams",
		sources: []string{bindingModel(`@sparse
map Params {
    key: String
    value: String
}

structure In {
    @httpQueryParams
    params: Params
}`, bindingOpInput)},
		substr: "sparse",
	},
	{
		name: "sparse_map_prefix_headers",
		sources: []string{bindingModel(`@sparse
map Params {
    key: String
    value: String
}

structure In {
    @httpPrefixHeaders("X-P-")
    headers: Params
}`, bindingOpInput)},
		substr: "sparse",
	},

	// D4: literal query parameters are not implemented, so they must be
	// rejected instead of silently generating a route that can never match.
	{
		name: "literal_query_uri",
		sources: []string{bindingModel("", `@http(method: "GET", uri: "/x?y=1")
operation Op {}`)},
		substr: "literal query",
	},

	// D8: HTTP-specific rules are owned by the binding index.
	{
		name: "duplicate_response_code",
		sources: []string{bindingModel(`structure Out {
    @httpResponseCode
    a: Integer

    @httpResponseCode
    b: Integer
}`, bindingOpOutput)},
		substr: "response code",
	},
	{
		name: "duplicate_error_code",
		sources: []string{
			`$version: "2.0"
namespace a

@error("client")
structure NotFound {
    message: String
}`,
			`$version: "2.0"
namespace b

@error("client")
structure NotFound {
    message: String
}`,
			`$version: "2.0"
namespace tdd

service S {
    version: "1.0.0"
    operations: [Op]
    rename: {
        "a#NotFound": "ANotFound"
    }
}

@http(method: "GET", uri: "/op")
operation Op {
    errors: [a#NotFound, b#NotFound]
}`,
		},
		substr: "NotFound",
	},
}

func TestBindingDangers(t *testing.T) {
	for _, tc := range bindingDangerCases {
		t.Run(tc.name, func(t *testing.T) {
			m := assembleMany(t, tc.sources...)
			_, diags := Index(m, model.MustShapeID(bindingTestService))
			requireDangerContaining(t, diags, tc.substr)
		})
	}
}

func bindingModel(shapes, op string) string {
	return `$version: "2.0"
namespace tdd

service S {
    version: "1.0.0"
    operations: [Op]
}

` + shapes + "\n\n" + op + "\n"
}

const (
	bindingOpInput = `@http(method: "POST", uri: "/op")
operation Op {
    input: In
}`

	bindingOpOutput = `@http(method: "POST", uri: "/op")
operation Op {
    output: Out
}`
)

func assembleMany(t *testing.T, sources ...string) *model.Model {
	t.Helper()
	files := make([]*idl.File, 0, len(sources))
	for i, src := range sources {
		file, err := idl.ParseWithOptions(
			fmt.Sprintf("binding-%d.smithy", i),
			src,
			idl.Options{Version: idl.V2_1, DefaultVersion: idl.V2_0},
		)
		if err != nil {
			t.Fatalf("parse source %d: %v", i, err)
		}
		files = append(files, file)
	}
	m, err := model.Assemble(files, model.Options{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return m
}

func requireDangerContaining(t *testing.T, diags []model.Diagnostic, substr string) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == model.SeverityDanger &&
			strings.Contains(strings.ToLower(d.FullMessage()), strings.ToLower(substr)) {
			return
		}
	}
	t.Fatalf("want a DANGER containing %q, got %v", substr, diags)
}
