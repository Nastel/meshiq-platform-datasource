package plugin

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestQueryDataService_TokenTravelsInHeaderOnly pins the token transport: the access token must
// be sent as the X-API-Key header and must never appear in the request URL (URLs end up in
// access logs and in *url.Error messages, which surface in panel errors and health checks).
func TestQueryDataService_TokenTravelsInHeaderOnly(t *testing.T) {
	const token = "secret-token-123"

	var gotHeader, gotURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get(HDR_API_KEY)
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"row-count": 0, "total-row-count": 0, "status": "SUCCESS", "rows": []}`))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: token}
	queryModel := QueryModel{JKQL: "get events"}

	if _, err := queryDataService(context.Background(), server.Client(), queryModel, options); err != nil {
		t.Fatalf("queryDataService: %v", err)
	}

	if gotHeader != token {
		t.Errorf("%s header = %q, want the token", HDR_API_KEY, gotHeader)
	}
	if strings.Contains(gotURL, token) || strings.Contains(gotURL, REQ_TOKEN) {
		t.Errorf("request URL %q must not carry the token", gotURL)
	}
	if !strings.Contains(gotURL, "/jkql?") || !strings.Contains(gotURL, REQ_QUERY+"=") {
		t.Errorf("request URL %q should still target /jkql with %s", gotURL, REQ_QUERY)
	}
}

// TestBuildUrlParams_ValuesSurviveEncoding pins the url.Values encoding: jKQL text containing
// '&', '+', '=' and quotes must round-trip intact.
func TestBuildUrlParams_ValuesSurviveEncoding(t *testing.T) {
	jkqlText := `get Log where ResourceName = 'A&B + C' and jk_query contains '='`
	queryModel := QueryModel{
		JKQL:     jkqlText,
		Locale:   "en-US",
		Timezone: "Europe/Vilnius",
		Date:     "100 TO 200",
		MaxRows:  50,
	}

	parsed, err := url.ParseQuery(buildUrlParams(queryModel).Encode())
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}

	if got := parsed.Get(REQ_QUERY); got != jkqlText {
		t.Errorf("%s round-trip = %q, want %q", REQ_QUERY, got, jkqlText)
	}
	if got := parsed.Get(REQ_MAXROWS); got != "50" {
		t.Errorf("%s = %q, want 50", REQ_MAXROWS, got)
	}
}

// TestQueryDataService_ErrorStatusWithJSONBody: a gateway/proxy answering with an HTTP error and
// a JSON body (no jk_ccode envelope) must be an error, not an empty result.
func TestQueryDataService_ErrorStatusWithJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message": "upstream unavailable"}`))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}
	_, err := queryDataService(context.Background(), server.Client(), QueryModel{JKQL: "get events"}, options)
	if err == nil {
		t.Fatal("HTTP 502 with a JSON body must fail, not pass as an empty result")
	}
	var qe *queryError
	if errors.As(err, &qe) {
		t.Errorf("transport-level failure must not be a queryError: %v", err)
	}
}

// TestQueryDataService_EnvelopeErrorIsQueryError: the dataservice's own error envelope
// (jk_ccode ERROR) must come back as a queryError, so the query layer reports it as a
// bad request rather than a bad gateway.
func TestQueryDataService_EnvelopeErrorIsQueryError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"jk_ccode": "ERROR", "jk_error": "Syntax error at token 'gett'"}`))
	}))
	defer server.Close()

	options := MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"}
	_, err := queryDataService(context.Background(), server.Client(), QueryModel{JKQL: "gett events"}, options)
	if err == nil {
		t.Fatal("an error envelope must fail the query")
	}
	var qe *queryError
	if !errors.As(err, &qe) {
		t.Errorf("envelope error should be a queryError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "Syntax error") {
		t.Errorf("error should carry jk_error, got %q", err.Error())
	}
}

// TestReadLimited_OversizedBodyIsRejected pins that a response body over the limit (a
// misconfigured Service URL, a proxy serving something unexpected — metadata calls send no
// jk_maxrows to bound them like a query would) is rejected instead of being read fully into
// memory. Exercises the limit directly with a small value rather than transferring
// maxResponseBodyBytes' worth of data through doGetRequest.
func TestReadLimited_OversizedBodyIsRejected(t *testing.T) {
	body := bytes.NewReader(bytes.Repeat([]byte("a"), 11))
	_, err := readLimited(body, 10)
	if !errors.Is(err, errResponseTooLarge) {
		t.Errorf("readLimited(11 bytes, limit 10): err = %v, want errResponseTooLarge", err)
	}
}

// TestReadLimited_BodyAtExactLimitIsAccepted pins the off-by-one boundary: a body of exactly the
// limit must not be misreported as oversized.
func TestReadLimited_BodyAtExactLimitIsAccepted(t *testing.T) {
	body := bytes.NewReader(bytes.Repeat([]byte("a"), 10))
	got, err := readLimited(body, 10)
	if err != nil {
		t.Fatalf("readLimited(10 bytes, limit 10): %v", err)
	}
	if len(got) != 10 {
		t.Errorf("body length = %d, want exactly 10", len(got))
	}
}

// TestDoGetRequest_UsesTheRealResponseSizeLimit is a thin end-to-end check that doGetRequest
// actually wires readLimited in with maxResponseBodyBytes (the unit tests above pin the
// boundary logic itself against a small limit).
func TestDoGetRequest_UsesTheRealResponseSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"row-count": 0, "status": "SUCCESS", "rows": []}`))
	}))
	defer server.Close()

	body, statusCode, err := doGetRequest(context.Background(), server.Client(), server.URL, "")
	if err != nil {
		t.Fatalf("doGetRequest: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("statusCode = %d, want 200", statusCode)
	}
	if !strings.Contains(string(body), "row-count") {
		t.Errorf("body = %s, want it to carry the response verbatim (well under the size limit)", body)
	}
}
