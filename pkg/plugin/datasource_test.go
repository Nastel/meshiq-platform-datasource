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
