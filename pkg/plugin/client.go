package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
)

// ServiceResponse is the raw result-set JSON returned by the dataservice /jkql endpoint.
type ServiceResponse map[string]interface{}

// queryError is a jKQL-level error reported by the dataservice error envelope (bad query,
// bad token), as opposed to a transport/HTTP failure. The query layer maps it to a
// bad-request response instead of a bad-gateway one.
type queryError struct{ message string }

func (e *queryError) Error() string { return e.message }

// testDataService runs a lightweight jKQL statement to verify connectivity/credentials.
func testDataService(ctx context.Context, httpClient *http.Client, options MeshIqDataSourceOptions) (ServiceResponse, error) {
	return queryDataService(ctx, httpClient, BuildParamsQueryModel(), options)
}

// queryDataService sends a jKQL query to the dataservice /jkql endpoint and returns the
// parsed result set.
func queryDataService(ctx context.Context, httpClient *http.Client, queryModel QueryModel, options MeshIqDataSourceOptions) (ServiceResponse, error) {
	requestUrl := buildRequestUrl(queryModel, options)

	bodyBytes, statusCode, err := doGetRequest(ctx, httpClient, requestUrl, options.Token)
	if err != nil {
		return nil, err
	}

	// TODO(dev-only): remove or comment out together with capture.go. No-op unless
	// MESHIQ_CAPTURE_DIR is set.
	captureResponse(queryModel, bodyBytes)

	response, err := parseServiceResponse(bodyBytes)
	if err != nil {
		// The body wasn't JSON — usually an HTML login/redirect or a proxy error page,
		// meaning the request didn't reach the jKQL endpoint. Surface the HTTP status and
		// a snippet instead of a cryptic "invalid character '<'" JSON error.
		return nil, buildNonJSONError(statusCode, bodyBytes)
	}

	if response[RESP_CCODE] == jkql.ERROR {
		return nil, &queryError{buildCustomErrorMessage(fmt.Sprint(response[RESP_ERROR]))}
	}

	// A JSON body without the error envelope can still ride an HTTP error status — a gateway
	// or proxy answering for the dataservice. Don't let it pass as an empty result.
	if statusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("dataservice returned HTTP %d: %s", statusCode, bodySnippet(bodyBytes))
	}

	return response, nil
}

// buildNonJSONError produces a readable error when the dataservice returns something other than
// the expected JSON result set (wrong Service URL, authentication redirect, reverse-proxy page…).
func buildNonJSONError(statusCode int, body []byte) error {
	return fmt.Errorf("dataservice returned a non-JSON response (HTTP %d) — check the Service URL and access token: %s", statusCode, bodySnippet(body))
}

// bodySnippet condenses a response body into a short single-line snippet for error messages.
func bodySnippet(body []byte) string {
	snippet := strings.TrimSpace(string(body))
	snippet = strings.Join(strings.Fields(snippet), " ") // collapse whitespace/newlines
	const max = 200
	if len(snippet) > max {
		snippet = snippet[:max] + "…"
	}
	if snippet == "" {
		snippet = "(empty response body)"
	}
	return snippet
}

func buildRequestUrl(queryModel QueryModel, options MeshIqDataSourceOptions) string {
	params := buildUrlParams(queryModel)
	return RemoveEndingSlash(options.ServiceUrl) + "/jkql?" + params.Encode()
}

// buildUrlParams builds the request's query parameters. url.Values escapes every value, so jKQL
// text containing '&', '+' or '=' survives intact and no value (including template-variable
// content) can inject extra jk_* parameters. The access token is NOT a URL parameter — it goes
// in the X-API-Key header (doGetRequest), so it never appears in access logs or in *url.Error
// messages shown to users.
func buildUrlParams(queryModel QueryModel) url.Values {
	params := url.Values{}

	params.Set(REQ_QUERY, strings.TrimLeft(queryModel.JKQL, " "))

	if queryModel.Locale != "" {
		params.Set(REQ_LOCALE, queryModel.Locale)
	}

	if queryModel.Timezone != "" {
		params.Set(REQ_TIMEZONE, queryModel.Timezone)
	}

	if queryModel.RepositoryID != "" {
		params.Set(REQ_REPO, queryModel.RepositoryID)
	}

	if queryModel.Date != "" {
		params.Set(REQ_DATE, queryModel.Date)
	}

	if queryModel.MaxRows != 0 {
		params.Set(REQ_MAXROWS, fmt.Sprintf("%d", queryModel.MaxRows))
	}

	if queryModel.Trace != nil && *queryModel.Trace {
		params.Set(REQ_TRACE, "true")
	}

	return params
}

// doGetRequest performs a GET bound to ctx (so panel cancels and alert deadlines propagate).
// apiKey, when non-empty, is sent as the X-API-Key header (the dataservice access token).
func doGetRequest(ctx context.Context, httpClient *http.Client, requestUrl string, apiKey string) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, 0, err
	}
	if apiKey != "" {
		request.Header.Set(HDR_API_KEY, apiKey)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(response.Body)

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, response.StatusCode, err
	}

	return bodyBytes, response.StatusCode, nil
}

func parseServiceResponse(bodyBytes []byte) (ServiceResponse, error) {
	response := make(ServiceResponse)
	// UseNumber keeps numbers as json.Number: jKQL INTEGER is a Java long, which float64
	// (plain Unmarshal) cannot represent exactly past 2^53. jkql.ToInt64/ToFloat64 read them.
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()
	err := decoder.Decode(&response)
	return response, err
}
