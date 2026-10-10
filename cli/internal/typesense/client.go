// Package typesense is a minimal client for the local Typesense server that
// `ipm serve` runs: health, collections, batched JSONL import and search.
// It speaks plain HTTP so ipm needs no client library.
package typesense

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrUnreachable is a server that does not answer: not started, or gone.
	ErrUnreachable = errors.New("typesense is not reachable")
	// ErrUnauthorized is a server that rejected the API key.
	ErrUnauthorized = errors.New("typesense rejected the API key")
	// ErrTimeout is a server that took too long to answer — busy, not gone.
	ErrTimeout = errors.New("typesense did not answer in time")
	// ErrMissingField is a search naming a field the collection doesn't have
	// (an index built by an older ipm).
	ErrMissingField = errors.New("typesense collection lacks a field")
	// ErrNoCollection is a request for a collection that does not exist.
	ErrNoCollection = errors.New("typesense has no such collection")
)

// Client talks to one Typesense server. A nil HTTP uses a client with a 30 s
// timeout; imports, which embed every document, allow 10 minutes.
type Client struct {
	BaseURL, APIKey string
	HTTP            *http.Client
}

func (c *Client) do(method, path string, query url.Values, body io.Reader, timeout time.Duration) ([]byte, error) {
	u := strings.TrimSuffix(c.BaseURL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-TYPESENSE-API-KEY", c.APIKey)
	hc := http.Client{Timeout: timeout}
	if c.HTTP != nil {
		hc = *c.HTTP
		if hc.Timeout == 0 {
			hc.Timeout = timeout
		}
	}
	resp, err := hc.Do(req)
	if err != nil {
		var ne net.Error
		var op *net.OpError
		switch {
		case errors.As(err, &ne) && ne.Timeout():
			return nil, fmt.Errorf("%w (after %s)", ErrTimeout, hc.Timeout)
		case errors.As(err, &op) && op.Op == "dial", errors.Is(err, io.EOF), strings.Contains(err.Error(), "connection refused"):
			return nil, fmt.Errorf("%w at %s: %v", ErrUnreachable, c.BaseURL, err)
		}
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return nil, ErrUnauthorized
	case resp.StatusCode == 404 && strings.Contains(string(data), "Could not find a field named"):
		return nil, fmt.Errorf("%w: %s", ErrMissingField, strings.TrimSpace(string(data)))
	case resp.StatusCode == 404 && strings.HasPrefix(path, "/collections/"):
		return nil, ErrNoCollection
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("typesense %s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// Health reports whether the server answers {"ok":true}.
func (c *Client) Health() error {
	data, err := c.do("GET", "/health", nil, nil, 5*time.Second)
	if err != nil {
		return err
	}
	var h struct{ OK bool }
	if json.Unmarshal(data, &h) != nil || !h.OK {
		return fmt.Errorf("typesense is not healthy yet: %s", strings.TrimSpace(string(data)))
	}
	return nil
}

// NumDocuments is how many documents a collection holds.
func (c *Client) NumDocuments(collection string) (int, error) {
	data, err := c.do("GET", "/collections/"+url.PathEscape(collection), nil, nil, 30*time.Second)
	if err != nil {
		return 0, err
	}
	var col struct {
		N int `json:"num_documents"`
	}
	err = json.Unmarshal(data, &col)
	return col.N, err
}

// DeleteCollection drops a collection; a missing one is not an error.
func (c *Client) DeleteCollection(name string) error {
	_, err := c.do("DELETE", "/collections/"+url.PathEscape(name), nil, nil, 30*time.Second)
	if errors.Is(err, ErrNoCollection) {
		return nil
	}
	return err
}

// CreateCollection creates a collection from its schema.
func (c *Client) CreateCollection(schema map[string]any) error {
	body, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	_, err = c.do("POST", "/collections", nil, bytes.NewReader(body), 30*time.Second)
	return err
}

// ImportResult is Typesense's answer for one imported document, in the
// order the documents were sent.
type ImportResult struct {
	Success  bool   `json:"success"`
	Code     int    `json:"code"`
	Error    string `json:"error"`
	Document string `json:"document"`
}

// Import creates documents from one JSONL request.
func (c *Client) Import(collection string, docs []map[string]any) ([]ImportResult, error) {
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetEscapeHTML(false)
	for _, d := range docs {
		if err := enc.Encode(d); err != nil {
			return nil, err
		}
	}
	data, err := c.do("POST", "/collections/"+url.PathEscape(collection)+"/documents/import",
		url.Values{"action": {"create"}}, &body, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	var out []ImportResult
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var r ImportResult
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("typesense import: %v", err)
		}
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(docs) {
		return nil, fmt.Errorf("typesense import answered %d result(s) for %d document(s)", len(out), len(docs))
	}
	return out, nil
}

// HybridInfo is how hybrid search ranked a hit: its rank-fusion score, 0–1,
// built from the hit's positions in the keyword and semantic rankings.
type HybridInfo struct {
	RankFusionScore float64 `json:"rank_fusion_score"`
}

// Hit is one search result. TextMatch is 0 for a semantic-only hit;
// VectorDistance and Hybrid are nil for keyword-only searches.
type Hit struct {
	Document       map[string]any `json:"document"`
	TextMatch      int64          `json:"text_match"`
	VectorDistance *float64       `json:"vector_distance,omitempty"`
	Hybrid         *HybridInfo    `json:"hybrid_search_info,omitempty"`
}

// FacetCount is one facet value and how many hits have it.
type FacetCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Result is a search response, facets keyed by field.
type Result struct {
	Found, SearchTimeMS int
	Hits                []Hit
	Facets              map[string][]FacetCount
}

// Search runs one search with the given parameters.
func (c *Client) Search(collection string, params url.Values) (Result, error) {
	data, err := c.do("GET", "/collections/"+url.PathEscape(collection)+"/documents/search", params, nil, 30*time.Second)
	if err != nil {
		return Result{}, err
	}
	var raw struct {
		Found        int   `json:"found"`
		SearchTimeMS int   `json:"search_time_ms"`
		Hits         []Hit `json:"hits"`
		FacetCounts  []struct {
			Field  string       `json:"field_name"`
			Counts []FacetCount `json:"counts"`
		} `json:"facet_counts"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Result{}, err
	}
	res := Result{Found: raw.Found, SearchTimeMS: raw.SearchTimeMS, Hits: raw.Hits, Facets: map[string][]FacetCount{}}
	for _, f := range raw.FacetCounts {
		res.Facets[f.Field] = f.Counts
	}
	return res, nil
}
