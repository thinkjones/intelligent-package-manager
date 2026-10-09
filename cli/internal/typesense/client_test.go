package typesense

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func serve(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client()}, srv
}

func TestHealth(t *testing.T) {
	c, srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path %s", r.URL.Path)
		}
		io.WriteString(w, `{"ok":true}`)
	})
	if err := c.Health(); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if err := c.Health(); !errors.Is(err, ErrUnreachable) {
		t.Errorf("closed server: %v", err)
	}
}

func TestRequestsCarryTheKey(t *testing.T) {
	c, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TYPESENSE-API-KEY") != "k" {
			t.Errorf("%s %s without key", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"ok":true,"num_documents":0,"found":0,"hits":[]}`)
	})
	c.Health()
	c.NumDocuments("artifacts")
	c.Search("artifacts", url.Values{"q": {"x"}})
}

func TestUnauthorized(t *testing.T) {
	c, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"message":"Forbidden"}`)
	})
	if _, err := c.NumDocuments("artifacts"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("got %v", err)
	}
	if _, err := c.Search("artifacts", url.Values{}); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("search got %v", err)
	}
}

func TestCollections(t *testing.T) {
	var created map[string]any
	c, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "DELETE" && r.URL.Path == "/collections/artifacts":
			w.WriteHeader(404)
		case r.Method == "POST" && r.URL.Path == "/collections":
			json.NewDecoder(r.Body).Decode(&created)
			io.WriteString(w, `{}`)
		case r.Method == "GET" && r.URL.Path == "/collections/artifacts":
			io.WriteString(w, `{"num_documents":42}`)
		case r.Method == "GET" && r.URL.Path == "/collections/nope":
			w.WriteHeader(404)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	if err := c.DeleteCollection("artifacts"); err != nil {
		t.Errorf("delete of a missing collection: %v", err)
	}
	if err := c.CreateCollection(map[string]any{"name": "artifacts"}); err != nil || created["name"] != "artifacts" {
		t.Errorf("create: %v %v", err, created)
	}
	if n, err := c.NumDocuments("artifacts"); err != nil || n != 42 {
		t.Errorf("num: %d %v", n, err)
	}
	if _, err := c.NumDocuments("nope"); !errors.Is(err, ErrNoCollection) {
		t.Errorf("missing: %v", err)
	}
}

func TestImport(t *testing.T) {
	c, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/artifacts/documents/import" || r.URL.Query().Get("action") != "create" {
			t.Errorf("%s ?%s", r.URL.Path, r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], `{"id":"a"`) {
			t.Errorf("body %q", body)
		}
		io.WriteString(w, "{\"success\":true}\n{\"code\":400,\"error\":\"bad\",\"success\":false}")
	})
	res, err := c.Import("artifacts", []map[string]any{{"id": "a"}, {"id": "b"}})
	if err != nil || len(res) != 2 || !res[0].Success || res[1].Success || res[1].Error != "bad" || res[1].Code != 400 {
		t.Errorf("got %+v %v", res, err)
	}
}

func TestSearch(t *testing.T) {
	c, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collections/nope/documents/search" {
			w.WriteHeader(404)
			return
		}
		if r.URL.Query().Get("q") != "unit tests" {
			t.Errorf("q %q", r.URL.Query().Get("q"))
		}
		io.WriteString(w, `{"found":3,"search_time_ms":7,"facet_counts":[{"field_name":"type","counts":[{"value":"skill","count":3}]}],
			"hits":[{"document":{"id":"skill:o/r:a#x","name":"tdd"},"text_match":578730123365711993,"vector_distance":0.55,"hybrid_search_info":{}}]}`)
	})
	res, err := c.Search("artifacts", url.Values{"q": {"unit tests"}})
	if err != nil || res.Found != 3 || res.SearchTimeMS != 7 || len(res.Hits) != 1 || res.Hits[0].Document["name"] != "tdd" ||
		res.Hits[0].TextMatch != 578730123365711993 || res.Hits[0].VectorDistance == nil || *res.Hits[0].VectorDistance != 0.55 ||
		len(res.Facets["type"]) != 1 || res.Facets["type"][0] != (FacetCount{Value: "skill", Count: 3}) {
		t.Errorf("got %+v %v", res, err)
	}
	if _, err := c.Search("nope", url.Values{}); !errors.Is(err, ErrNoCollection) {
		t.Errorf("missing: %v", err)
	}
}
