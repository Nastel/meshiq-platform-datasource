package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
)

// TestFetchFunctionCatalog_GroupsByType verifies the Name/Type columns of a `get functions`
// result set are grouped into the catalog's aggregate/analytic/scalar buckets, and that a name
// reported under more than one type is placed once, preferring Aggregate. The grouping is
// exercised indirectly through jkql.BuildDataModel (FunctionCatalog has no public inspection
// API): an aggregate-over-map header only explodes per key when the catalog recognizes "Avg" as
// an aggregate name.
func TestFetchFunctionCatalog_GroupsByType(t *testing.T) {
	const raw = `{
		"row-count": 3, "total-row-count": 3, "status": "SUCCESS",
		"colhdr": ["Name", "Type"],
		"coltype": {"Name": "STRING", "Type": "STRING"},
		"collabel": {"Name": "Name", "Type": "Type"},
		"rows": [
			{"Name": "Avg", "Type": "Aggregate"},
			{"Name": "Avg", "Type": "Analytic"},
			{"Name": "Length", "Type": "Scalar"}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}
	cat, err := fetchFunctionCatalog(context.Background(), server.Client(), options)
	if err != nil {
		t.Fatalf("fetchFunctionCatalog: %v", err)
	}
	if cat == nil {
		t.Fatal("expected a catalog, got nil")
	}

	const aggResult = `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Avg(Quota)"],
		"coltype": {"Avg(Quota)": "MAP(DECIMAL)"},
		"collabel": {"Avg(Quota)": "Avg(Quota)"},
		"rows": [{"Avg(Quota)": {"A": 1.5, "B": 2.5}}]
	}`
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(aggResult), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	model := jkql.BuildDataModel(parsed, cat)
	if len(model.Headers) != 2 {
		t.Fatalf("expected Avg(Quota) to explode into 2 columns (one per map key), got %d: %v", len(model.Headers), model.Headers)
	}
}

// TestFetchFunctionCatalog_FallsBackOnError verifies a transport failure or an empty/unusable
// response returns nil, so the caller falls back to the hardcoded default catalog. It also
// returns the underlying error, which the caller (functionCatalog) uses to decide whether the
// negative is safe to cache — a queryError envelope (the dataservice DID answer) is.
func TestFetchFunctionCatalog_FallsBackOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"jk_ccode": "ERROR", "jk_error": "no such statement"}`))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}
	cat, err := fetchFunctionCatalog(context.Background(), server.Client(), options)
	if cat != nil {
		t.Error("expected nil catalog on a dataservice error, so the caller falls back")
	}
	var qe *queryError
	if !errors.As(err, &qe) {
		t.Errorf("a dataservice error envelope should surface as a queryError, got %T: %v", err, err)
	}
}

// TestDatasource_FunctionCatalog_CachesAndFallsBack verifies the datasource-level cache: a genuine
// "get functions" failure answer (a queryError envelope — the dataservice DID answer) is replaced
// with the hardcoded default and never retried (the second call must not hit the server again).
func TestDatasource_FunctionCatalog_CachesAndFallsBack(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"jk_ccode": "ERROR", "jk_error": "no such statement"}`))
	}))
	defer server.Close()

	d := &Datasource{httpClient: server.Client()}
	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}

	first := d.functionCatalog(context.Background(), options)
	second := d.functionCatalog(context.Background(), options)

	if first == nil || second == nil {
		t.Fatal("functionCatalog must never return nil (it falls back to the default)")
	}
	if first != second {
		t.Error("expected the same cached catalog instance on the second call")
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 dataservice call, got %d", calls)
	}
}

// TestDatasource_FunctionCatalog_DoesNotCacheATransportFailure verifies a fetch that failed at
// the transport level (here, a bare HTTP 500 with no dataservice error envelope — the server
// never actually answered) is not cached: unlike TestDatasource_FunctionCatalog_CachesAndFallsBack
// above, the second call must retry rather than reuse a pinned fallback.
func TestDatasource_FunctionCatalog_DoesNotCacheATransportFailure(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := &Datasource{httpClient: server.Client()}
	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}

	d.functionCatalog(context.Background(), options)
	d.functionCatalog(context.Background(), options)

	if calls != 2 {
		t.Errorf("expected 2 dataservice calls (a transport failure must not be cached), got %d", calls)
	}
}

// TestDatasource_FunctionCatalog_DoesNotCacheOnContextCancellation verifies a fetch that failed
// only because the caller's context ended is not cached: the next call with a live context must
// retry the load instead of being stuck with the fallback catalog for the datasource's lifetime.
func TestDatasource_FunctionCatalog_DoesNotCacheOnContextCancellation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
			"colhdr": ["Name", "Type"], "coltype": {"Name": "STRING", "Type": "STRING"},
			"collabel": {"Name": "Name", "Type": "Type"},
			"rows": [{"Name": "Avg", "Type": "Aggregate"}]
		}`))
	}))
	defer server.Close()

	d := &Datasource{httpClient: server.Client()}
	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	first := d.functionCatalog(canceledCtx, options)
	if first == nil {
		t.Fatal("functionCatalog must never return nil (it falls back to the default)")
	}

	second := d.functionCatalog(context.Background(), options)
	if second == nil {
		t.Fatal("functionCatalog must never return nil")
	}
	// The canceled call's request never reaches the server (it fails client-side before dialing),
	// so exactly 1 call confirms the retry actually happened rather than reusing a cached fallback
	// — a cached-negative bug would also pass the nil checks above without ever incrementing calls.
	if calls != 1 {
		t.Errorf("expected the live-context retry to reach the server exactly once, got %d", calls)
	}
}
