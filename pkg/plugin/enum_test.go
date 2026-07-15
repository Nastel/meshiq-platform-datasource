package plugin

import (
	"context"
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
	text := fetchEnumValues(context.Background(), server.Client(), "Severity", options)

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
// result's own ordinals.
func TestFetchEnumValues_NilOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"jk_ccode": "ERROR", "jk_error": "not an enum field"}`))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}
	if text := fetchEnumValues(context.Background(), server.Client(), "NotAnEnum", options); text != nil {
		t.Errorf("expected nil on dataservice error, got %v", text)
	}
}

// TestDatasource_EnumValues_CachesResult verifies enumValues memoizes on the datasource: a second
// call for the same field must not hit the server again, and a failed lookup is cached as an
// empty (not nil) slice so it isn't retried either.
func TestDatasource_EnumValues_CachesResult(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
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
