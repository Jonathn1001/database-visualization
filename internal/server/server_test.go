package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elgnas/dbviz/internal/audit"
	"github.com/elgnas/dbviz/internal/connection"
)

func testRouter(cfg Config) http.Handler {
	return NewRouter(cfg, nil, connection.NewManager(connection.Config{}, nil), audit.Nop{})
}

func TestHealthEndpoint(t *testing.T) {
	r := testRouter(Config{Dev: true, Version: "test"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status field = %q, want ok", body["status"])
	}
}

func TestVersionEndpoint(t *testing.T) {
	r := testRouter(Config{Dev: true, Version: "v1.2.3"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/version")
	if err != nil {
		t.Fatalf("GET /api/version: %v", err)
	}
	defer res.Body.Close()

	var body map[string]string
	_ = json.NewDecoder(res.Body).Decode(&body)
	if body["version"] != "v1.2.3" {
		t.Fatalf("version = %q, want v1.2.3", body["version"])
	}
}
