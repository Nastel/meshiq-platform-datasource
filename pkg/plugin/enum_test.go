package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFetchEnumValues_BuildsDenseTable verifies the (ID, Name) rows of a "GET ENUMERATION FOR"
// result set are placed into a table indexed by ordinal, sized to the largest ordinal seen.
func TestFetchEnumValues_BuildsDenseTable(t *testing.T) {
	const raw = `{
		"row-count": 3, "total-row-count": 3, "status": "SUCCESS",
		"colhdr": ["ID", "Name"],
		"coltype": {"ID": "INTEGER", "Name": "STRING"},
		"collabel": {"ID": "ID", "Name": "Name"},
		"rows": [
			{"ID": 0, "Name": "NONE"},
			{"ID": 1, "Name": "INFO"},
			{"ID": 3, "Name": "ERROR"}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}
	text, err := fetchEnumValues(context.Background(), server.Client(), "Severity", options)
	if err != nil {
		t.Fatalf("fetchEnumValues: %v", err)
	}

	want := []string{"NONE", "INFO", "", "ERROR"} // ordinal 2 has no row, so it's an empty gap
	if len(text) != len(want) {
		t.Fatalf("got %v, want %v", text, want)
	}
	for i, w := range want {
		if text[i] != w {
			t.Errorf("text[%d] = %q, want %q", i, text[i], w)
		}
	}
}

// TestFetchEnumValues_NilOnError verifies a dataservice failure (not an enumerable field, or a
// transport error) returns nil so the caller falls back to the compact table built from the
// result's own ordinals. It also returns the underlying error, which the caller (enumValues) uses
// to decide whether the negative is safe to cache — see TestFetchEnumValues_ErrorIsAQueryError
// and TestDatasource_EnumValues_DoesNotCacheATransportFailure below.
func TestFetchEnumValues_NilOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"jk_ccode": "ERROR", "jk_error": "not an enum field"}`))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}
	text, err := fetchEnumValues(context.Background(), server.Client(), "NotAnEnum", options)
	if text != nil {
		t.Errorf("expected nil on dataservice error, got %v", text)
	}
	var qe *queryError
	if !errors.As(err, &qe) {
		t.Errorf("a dataservice error envelope should surface as a queryError, got %T: %v", err, err)
	}
}

// TestDatasource_EnumValues_CachesResult verifies enumValues memoizes on the datasource: a second
// call for the same field must not hit the server again, and a genuine "not an enum field" answer
// (a queryError envelope — the dataservice DID answer) is cached as an empty (not nil) slice so
// it isn't retried either.
func TestDatasource_EnumValues_CachesResult(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"jk_ccode": "ERROR", "jk_error": "not an enum field"}`))
	}))
	defer server.Close()

	d := &Datasource{httpClient: server.Client(), enumCache: make(map[string][]string)}
	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}

	first := d.enumValues(context.Background(), "Severity", options)
	second := d.enumValues(context.Background(), "Severity", options)

	if first == nil || second == nil {
		t.Fatal("enumValues must never return nil (a failed lookup caches as an empty slice)")
	}
	if len(first) != 0 || len(second) != 0 {
		t.Errorf("expected an empty slice for a failed lookup, got %v / %v", first, second)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 dataservice call, got %d", calls)
	}
}

// TestDatasource_EnumValues_DoesNotCacheATransportFailure verifies a fetch that failed at the
// transport level (here, a bare HTTP 500 with no dataservice error envelope — the server never
// actually answered "not an enum field") is not cached: unlike TestDatasource_EnumValues_
// CachesResult above, the second call must retry rather than reuse a pinned empty result.
func TestDatasource_EnumValues_DoesNotCacheATransportFailure(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := &Datasource{httpClient: server.Client(), enumCache: make(map[string][]string)}
	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}

	d.enumValues(context.Background(), "Severity", options)
	d.enumValues(context.Background(), "Severity", options)

	if calls != 2 {
		t.Errorf("expected 2 dataservice calls (a transport failure must not be cached), got %d", calls)
	}
}

// TestDatasource_EnumValues_DoesNotCacheOnContextCancellation verifies a fetch that failed only
// because the caller's context ended is not cached: the field must be retried on the next call
// with a live context, unlike a real "not an enum field" answer.
func TestDatasource_EnumValues_DoesNotCacheOnContextCancellation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
			"colhdr": ["ID", "Name"], "coltype": {"ID": "INTEGER", "Name": "STRING"},
			"collabel": {"ID": "ID", "Name": "Name"},
			"rows": [{"ID": 0, "Name": "NONE"}]
		}`))
	}))
	defer server.Close()

	d := &Datasource{httpClient: server.Client(), enumCache: make(map[string][]string)}
	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	first := d.enumValues(canceledCtx, "Severity", options)
	if len(first) != 0 {
		t.Fatalf("expected an empty result for the canceled call, got %v", first)
	}

	second := d.enumValues(context.Background(), "Severity", options)
	if len(second) != 1 || second[0] != "NONE" {
		t.Errorf("expected the retry to succeed with a live context, got %v", second)
	}
	// The canceled call's request never reaches the server (it fails client-side before dialing),
	// so exactly 1 call confirms the retry actually happened rather than returning a cached empty
	// result — a cached-negative bug would also show len(second) == 0 without ever incrementing calls.
	if calls != 1 {
		t.Errorf("expected the live-context retry to reach the server exactly once, got %d", calls)
	}
}

// TestFetchEnumValues_ClampsRunawayOrdinal guards against a real allocation risk: an ID column
// that isn't actually an enum ordinal (wrong field, bad response) used to size the table directly
// off whatever value it saw, up to an out-of-memory or makeslice panic. A too-large ordinal must
// be skipped (and recorded as an issue) instead of growing the table.
func TestFetchEnumValues_ClampsRunawayOrdinal(t *testing.T) {
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["ID", "Name"], "coltype": {"ID": "INTEGER", "Name": "STRING"},
		"collabel": {"ID": "ID", "Name": "Name"},
		"rows": [
			{"ID": 0, "Name": "NONE"},
			{"ID": 1000000000, "Name": "BOGUS"}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}
	text, err := fetchEnumValues(context.Background(), server.Client(), "Severity", options)
	if err != nil {
		t.Fatalf("fetchEnumValues: %v", err)
	}

	if len(text) != 1 || text[0] != "NONE" {
		t.Fatalf("got %v, want the runaway ordinal skipped and the table sized to the remaining rows", text)
	}
}
