package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func parseResultSet(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal result set: %v", err)
	}
	return m
}

// TestFieldsExtraction mirrors handleFields: it verifies that FieldName/DataType are read and that
// the custom flag is taken from the exploded Properties('isCustom') column (jkql.BuildDataModel
// explodes the Properties MAP, so the flag is not a raw-map value on the row).
func TestFieldsExtraction(t *testing.T) {
	const raw = `{
		"row-count": 2, "total-row-count": 2,
		"item-type": "Field", "status": "SUCCESS",
		"colhdr": ["FieldName", "DataType", "Properties"],
		"coltype": {"FieldName": "STRING", "DataType": "ENUM", "Properties": "MAP"},
		"collabel": {"FieldName": "FieldName", "DataType": "DataType", "Properties": "Properties"},
		"rows": [
			{"FieldName": "Severity", "DataType": "7#ENUM",
			 "Properties": {"isCustom": false}, "Properties:_ValueTypes": {"isCustom": "BOOLEAN"}},
			{"FieldName": "Region", "DataType": "0#STRING",
			 "Properties": {"isCustom": true}, "Properties:_ValueTypes": {"isCustom": "BOOLEAN"}}
		]
	}`

	model := jkql.BuildDataModel(parseResultSet(t, raw), nil)

	customHeader := findHeaderByName(model, "Properties('isCustom')")
	if customHeader == "" {
		t.Fatal("expected an exploded Properties('isCustom') column")
	}

	type got struct {
		name, dataType string
		custom         bool
	}
	var results []got
	for _, row := range model.Rows {
		name, ok := grafanaString(model, row, jkql.FIELD_NAME)
		if !ok {
			continue
		}
		dataType, _ := grafanaString(model, row, jkql.DATA_TYPE)
		custom, _ := row[customHeader].(bool)
		results = append(results, got{name, dataType, custom})
	}

	want := []got{
		{"Severity", "ENUM", false},
		{"Region", "STRING", true},
	}
	if len(results) != len(want) {
		t.Fatalf("got %d fields, want %d: %+v", len(results), len(want), results)
	}
	for i, w := range want {
		if results[i] != w {
			t.Errorf("field %d = %+v, want %+v", i, results[i], w)
		}
	}
}

// TestTablesExtraction verifies /tables reads the ItemName column from a "get items" result set,
// including when ItemType is a LABELSET.
func TestTablesExtraction(t *testing.T) {
	const raw = `{
		"row-count": 3, "total-row-count": 3,
		"item-type": "Item", "status": "SUCCESS",
		"colhdr": ["ItemName", "ItemType"],
		"coltype": {"ItemName": "STRING", "ItemType": "LABELSET"},
		"collabel": {"ItemName": "ItemName", "ItemType": "ItemType"},
		"rows": [
			{"ItemName": "Log", "ItemType": "LOG"},
			{"ItemName": "Event", "ItemType": "EVENT"},
			{"ItemName": "Snapshot", "ItemType": "SNAPSHOT"}
		]
	}`

	model := jkql.BuildDataModel(parseResultSet(t, raw), nil)
	names := collectColumnStrings(model, jkql.ITEM_NAME)

	want := []string{"Log", "Event", "Snapshot"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("item %d = %q, want %q", i, names[i], w)
		}
	}
}

// TestMetadataQueryErrorStatus pins that a metadata-query failure is classified by its actual
// cause, not flattened to one status: invalid local config and a dataservice-reported rejection
// are both client-side problems (400); only a transport-level failure to even reach the
// dataservice counts as a gateway/downstream outage (502).
func TestMetadataQueryErrorStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"invalid options", fmt.Errorf("%w: %w", errInvalidOptions, errors.New("invalid access token")), http.StatusBadRequest},
		{"dataservice rejected the query", &queryError{message: "bad jKQL"}, http.StatusBadRequest},
		{"transport failure", errors.New("connection refused"), http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metadataQueryErrorStatus(tt.err); got != tt.want {
				t.Errorf("metadataQueryErrorStatus(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// callResource drives d.CallResource through the real httpadapter/mux (not the extraction helpers
// directly), returning the status and decoded JSON body. path must include the leading slash and
// any query string, e.g. "/fields?table=Log". serviceUrl becomes the datasource's configured
// Service URL (JSONData), so a call that reaches queryDataService hits whatever server the caller
// set up (or a deliberately unroutable address, to force a transport failure).
func callResource(t *testing.T, d *Datasource, serviceUrl, path string) (int, []byte) {
	t.Helper()
	settings := &backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"serviceUrl":"` + serviceUrl + `"}`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "t"},
	}
	req := &backend.CallResourceRequest{
		PluginContext: backend.PluginContext{DataSourceInstanceSettings: settings},
		Method:        http.MethodGet,
		Path:          path,
	}
	rec := &resourceResponseRecorder{}
	if err := d.CallResource(context.Background(), req, rec); err != nil {
		t.Fatalf("CallResource: %v", err)
	}
	if rec.response == nil {
		t.Fatal("CallResource never sent a response")
	}
	return rec.response.Status, rec.response.Body
}

type resourceResponseRecorder struct {
	response *backend.CallResourceResponse
}

func (r *resourceResponseRecorder) Send(resp *backend.CallResourceResponse) error {
	r.response = resp
	return nil
}

// TestHandleFields_InvalidTableIsBadRequest exercises the actual HTTP layer (mux + httpadapter),
// not just the extraction helpers: an invalid "table" query parameter must be rejected before any
// dataservice call, as a 400 with a JSON error body — not the 502 used for a real downstream
// failure.
func TestHandleFields_InvalidTableIsBadRequest(t *testing.T) {
	d := &Datasource{httpClient: http.DefaultClient, enumCache: make(map[string][]string)}
	d.resourceHandler = d.newResourceHandler()

	status, body := callResource(t, d, "http://unused", "/fields?table=not valid; DROP")

	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
	var errBody map[string]string
	if err := json.Unmarshal(body, &errBody); err != nil {
		t.Fatalf("error body isn't valid JSON: %v (%s)", err, body)
	}
	if errBody["error"] == "" {
		t.Errorf("error body = %v, want a non-empty \"error\" key", errBody)
	}
}

// TestHandleFields_DataserviceSuccessReturns200 is an end-to-end check with a real backing
// dataservice (httptest server): the resource path returns 200 with the expected JSON shape.
func TestHandleFields_DataserviceSuccessReturns200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
			"colhdr": ["FieldName", "DataType", "Properties"],
			"coltype": {"FieldName": "STRING", "DataType": "ENUM", "Properties": "MAP"},
			"collabel": {"FieldName": "FieldName", "DataType": "DataType", "Properties": "Properties"},
			"rows": [{"FieldName": "Severity", "DataType": "7#ENUM",
				"Properties": {"isCustom": false}, "Properties:_ValueTypes": {"isCustom": "BOOLEAN"}}]
		}`))
	}))
	defer server.Close()

	d := &Datasource{httpClient: server.Client(), enumCache: make(map[string][]string)}
	d.resourceHandler = d.newResourceHandler()

	status, body := callResource(t, d, server.URL, "/fields?table=Log")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", status, http.StatusOK, body)
	}
	var fields []fieldInfo
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("body isn't the expected []fieldInfo JSON: %v (%s)", err, body)
	}
	want := []fieldInfo{{Name: "Severity", Type: "ENUM", Custom: false}}
	if len(fields) != len(want) || fields[0] != want[0] {
		t.Errorf("fields = %+v, want %+v", fields, want)
	}
}

// TestHandleFields_DataserviceUnreachableIsBadGateway pins that an actual transport failure (the
// dataservice can't be reached at all) is reported as 502, distinct from the 400 used for a local
// validation problem or a dataservice-side query rejection.
func TestHandleFields_DataserviceUnreachableIsBadGateway(t *testing.T) {
	d := &Datasource{httpClient: http.DefaultClient, enumCache: make(map[string][]string)}
	d.resourceHandler = d.newResourceHandler()

	status, _ := callResource(t, d, "http://127.0.0.1:0", "/fields?table=Log")

	if status != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", status, http.StatusBadGateway)
	}
}
