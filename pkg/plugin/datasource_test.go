package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func TestQueryData(t *testing.T) {
	ds := Datasource{}

	resp, err := ds.QueryData(
		context.Background(),
		&backend.QueryDataRequest{
			Queries: []backend.DataQuery{
				{RefID: "A"},
			},
		},
	)
	if err != nil {
		t.Error(err)
	}

	if len(resp.Responses) != 1 {
		t.Fatal("QueryData must return a response")
	}
}

// TestQuery_BackendOnlyQueryFallsBackToDefaultRepository pins the alerting path: a query that
// arrives without a repository (an alert rule or other backend-only caller that bypasses the
// frontend's required repository selector) must fall back to the datasource's configured default
// repository, not fail or query with no repository at all.
func TestQuery_BackendOnlyQueryFallsBackToDefaultRepository(t *testing.T) {
	var gotRepo string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRepo = r.URL.Query().Get(REQ_REPO)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"row-count": 0, "total-row-count": 0, "status": "SUCCESS", "colhdr": [], "coltype": {}, "collabel": {}, "rows": []}`))
	}))
	defer server.Close()

	ds := &Datasource{httpClient: server.Client(), enumCache: make(map[string][]string)}

	settings := backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"serviceUrl":"` + server.URL + `","repositoryID":"DefaultRepo$Org"}`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "t"},
	}
	pCtx := backend.PluginContext{DataSourceInstanceSettings: &settings}

	// Simulate a backend-only query (e.g. an alert rule): no repositoryID on the query itself.
	query := backend.DataQuery{RefID: "A", JSON: []byte(`{"jkql":"get events"}`)}

	resp := ds.query(context.Background(), pCtx, query)
	if resp.Error != nil {
		t.Fatalf("query failed: %v", resp.Error)
	}

	decodedRepo, err := url.QueryUnescape(gotRepo)
	if err != nil {
		t.Fatalf("QueryUnescape: %v", err)
	}
	if decodedRepo != "DefaultRepo$Org" {
		t.Errorf("dataservice request repo = %q, want the datasource default %q", decodedRepo, "DefaultRepo$Org")
	}
}

// TestRejectCrossHostRedirect_SameHostAllowed pins that a same-host, same-scheme redirect (e.g.
// the dataservice itself issuing a 307 for its own reasons) is still followed.
func TestRejectCrossHostRedirect_SameHostAllowed(t *testing.T) {
	initial, err := url.Parse("https://dataservice.example.com/jkql")
	if err != nil {
		t.Fatal(err)
	}
	next, err := url.Parse("https://dataservice.example.com/jkql?retry=1")
	if err != nil {
		t.Fatal(err)
	}

	err = rejectCrossHostRedirect(&http.Request{URL: next}, []*http.Request{{URL: initial}})
	if err != nil {
		t.Errorf("same-host redirect must be allowed, got error: %v", err)
	}
}

// TestRejectCrossHostRedirect_SchemeDowngradeRefused pins that a same-host redirect from https to
// http is refused too, not just a cross-host one — otherwise the X-API-Key header would be replayed
// in cleartext to a same-host target that merely swapped scheme (e.g. a misconfigured redirect or a
// captive/login page terminating TLS early).
func TestRejectCrossHostRedirect_SchemeDowngradeRefused(t *testing.T) {
	initial, err := url.Parse("https://dataservice.example.com/jkql")
	if err != nil {
		t.Fatal(err)
	}
	next, err := url.Parse("http://dataservice.example.com/jkql")
	if err != nil {
		t.Fatal(err)
	}

	err = rejectCrossHostRedirect(&http.Request{URL: next}, []*http.Request{{URL: initial}})
	if err == nil {
		t.Error("same-host https-to-http downgrade redirect must be refused")
	}
}

// TestRejectCrossHostRedirect_CrossHostRefused pins the actual security property: a redirect to a
// different host must be refused, so the X-API-Key header (which http.Client would otherwise
// forward unchanged, unlike Authorization/Cookie) never reaches a host other than the configured
// Service URL's.
func TestRejectCrossHostRedirect_CrossHostRefused(t *testing.T) {
	initial, err := url.Parse("https://dataservice.example.com/jkql")
	if err != nil {
		t.Fatal(err)
	}
	next, err := url.Parse("https://attacker.example.com/jkql")
	if err != nil {
		t.Fatal(err)
	}

	err = rejectCrossHostRedirect(&http.Request{URL: next}, []*http.Request{{URL: initial}})
	if err == nil {
		t.Error("cross-host redirect must be refused")
	}
}

// TestQueryDataService_CrossHostRedirectRefused is an end-to-end check with a real redirecting
// server: the dataservice client must not follow a redirect to a different host, and the token
// must never be sent to that other host.
func TestQueryDataService_CrossHostRedirectRefused(t *testing.T) {
	const token = "secret-token-123"
	var attackerGotHeader string
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerGotHeader = r.Header.Get(HDR_API_KEY)
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/jkql", http.StatusFound)
	}))
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = rejectCrossHostRedirect

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: token}
	queryModel := QueryModel{JKQL: "get events"}

	_, err := queryDataService(context.Background(), client, queryModel, options)
	if err == nil {
		t.Fatal("expected the cross-host redirect to be refused, got no error")
	}
	if attackerGotHeader != "" {
		t.Errorf("token must never reach the redirect target, got %s header %q", HDR_API_KEY, attackerGotHeader)
	}
}

// TestCheckHealth_CompletionEnabledWithoutUrlFails pins that enabling completion with no
// completion service URL configured fails the health check, instead of quietly reporting a
// healthy datasource while the query editor gets no suggestions.
func TestCheckHealth_CompletionEnabledWithoutUrlFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"row-count": 0, "total-row-count": 0, "status": "SUCCESS", "colhdr": [], "coltype": {}, "collabel": {}, "rows": []}`))
	}))
	defer server.Close()

	ds := &Datasource{httpClient: server.Client(), enumCache: make(map[string][]string)}
	settings := backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"serviceUrl":"` + server.URL + `","enableCompletion":true}`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "t"},
	}

	result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{
		PluginContext: backend.PluginContext{DataSourceInstanceSettings: &settings},
	})
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if result.Status != backend.HealthStatusError {
		t.Errorf("status = %v, want HealthStatusError (completion enabled with no URL configured)", result.Status)
	}
}
