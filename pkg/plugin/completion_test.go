package plugin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// TestHandleSuggestions_InvalidPositionIsBadRequest pins that a malformed jk_position — the
// caller's own mistake, the request never reaches the completion service — is reported as a 400,
// not the 502 used for an actual downstream/service failure.
func TestHandleSuggestions_InvalidPositionIsBadRequest(t *testing.T) {
	settings := backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"serviceUrl":"http://unused","completionServiceUrl":"http://unused","enableCompletion":true}`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "t"},
	}
	pCtx := backend.PluginContext{DataSourceInstanceSettings: &settings}

	ds := &Datasource{httpClient: http.DefaultClient}

	req := httptest.NewRequest(http.MethodGet, "/suggestions?jk_query=get+events&jk_position=not-a-number", nil)
	req = req.WithContext(backend.WithPluginContext(req.Context(), pCtx))
	rec := httptest.NewRecorder()

	ds.handleSuggestions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (a malformed jk_position never reaches the completion service)", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleSuggestions_ServiceRejectionIsQuiet pins that a completion-service rejection (e.g. the
// jKQL query is momentarily invalid mid-typing) still answers 502 with a JSON error body — the
// quiet-logging change (writeResourceErrorQuiet, so this doesn't warn on every keystroke) must not
// change the response contract, only whether it's logged.
func TestHandleSuggestions_ServiceRejectionIsQuiet(t *testing.T) {
	completionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`invalid query`))
	}))
	defer completionServer.Close()

	settings := backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"serviceUrl":"http://unused","completionServiceUrl":"` + completionServer.URL + `","enableCompletion":true}`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "t"},
	}
	pCtx := backend.PluginContext{DataSourceInstanceSettings: &settings}

	ds := &Datasource{httpClient: completionServer.Client()}

	req := httptest.NewRequest(http.MethodGet, "/suggestions?jk_query=get+events+where&jk_position=17", nil)
	req = req.WithContext(backend.WithPluginContext(req.Context(), pCtx))
	rec := httptest.NewRecorder()

	ds.handleSuggestions(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body isn't valid JSON: %v (%s)", err, rec.Body.String())
	}
	if body["error"] == "" {
		t.Errorf("error body = %v, want a non-empty \"error\" key", body)
	}
}

// TestHandleSuggestions_SendsAFormEncodedPost pins the wire contract with the real autocomplete
// service (AutoCompleteMsgHandler): it accepts POST, but treats the body as the same url-encoded
// param string a GET would carry in its query string — not JSON. The outbound request must be a
// POST with a bare (query-string-free) URL and jk_query/jk_position/jk_repo url-encoded into the
// body as application/x-www-form-urlencoded.
func TestHandleSuggestions_SendsAFormEncodedPost(t *testing.T) {
	var gotMethod, gotURL, gotContentType, gotBody string
	completionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotURL = r.URL.String()
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer completionServer.Close()

	settings := backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"serviceUrl":"http://unused","completionServiceUrl":"` + completionServer.URL + `","enableCompletion":true}`),
		DecryptedSecureJSONData: map[string]string{"accessToken": "t"},
	}
	pCtx := backend.PluginContext{DataSourceInstanceSettings: &settings}

	ds := &Datasource{httpClient: completionServer.Client()}

	req := httptest.NewRequest(http.MethodGet, "/suggestions?jk_query=get+events&jk_position=5", nil)
	req = req.WithContext(backend.WithPluginContext(req.Context(), pCtx))
	rec := httptest.NewRecorder()

	ds.handleSuggestions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if strings.Contains(gotURL, "?") {
		t.Errorf("request URL = %q, want no query string (the long jk_query goes in the body, not the URL)", gotURL)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotContentType)
	}
	values, err := url.ParseQuery(gotBody)
	if err != nil {
		t.Fatalf("body %q did not parse as a url-encoded form: %v", gotBody, err)
	}
	if values.Get("jk_query") != "get events" || values.Get("jk_position") != "5" {
		t.Errorf("body = %q, want jk_query=get events and jk_position=5", gotBody)
	}
}
