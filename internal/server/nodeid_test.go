package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestUnescapeNodeID guards the fix for the schema-qualified node ID bug: the
// frontend percent-encodes the "." separator as %2E, but Go's net/http does not
// decode %2E in request paths, so handlers must unescape it themselves before
// splitting "<schema>.<table>".
func TestUnescapeNodeID(t *testing.T) {
	cases := map[string]string{
		"public%2Eusers":     "public.users",    // %2E -> .
		"audit%2Elogin_log":  "audit.login_log", // underscores survive
		"public.users":       "public.users",    // literal dot passes through
		"noschema":           "noschema",        // engines without schemas
		"public%2Eorder%2Ex": "public.order.x",  // multiple escapes
		"%zz":                "%zz",             // invalid escape -> raw fallback
	}
	for in, want := range cases {
		if got := unescapeNodeID(in); got != want {
			t.Errorf("unescapeNodeID(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDecodeNodeIDThroughRouter exercises the fix at the real HTTP/chi routing
// surface — the layer that actually produced the bug. Go's net/http leaves %2E
// encoded in the path, so before the fix chi.URLParam returned "public%2Eusers"
// and the handler rejected it; decodeNodeID must yield the decoded id.
func TestDecodeNodeIDThroughRouter(t *testing.T) {
	var got string
	r := chi.NewRouter()
	r.Get("/api/connections/{id}/tables/{nodeId}/sample", func(_ http.ResponseWriter, req *http.Request) {
		got = decodeNodeID(req)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/connections/abc/tables/public%2Eusers/sample")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got != "public.users" {
		t.Fatalf("decodeNodeID through chi routing = %q, want %q", got, "public.users")
	}
}
