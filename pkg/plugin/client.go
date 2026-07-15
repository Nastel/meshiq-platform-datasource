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
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
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
		// meaning the request didn't reach the jKQL endpoint. Log the raw body server-side
		// (it can contain proxy/login-page HTML not fit for a panel error) and surface the
		// HTTP status instead of a cryptic "invalid character '<'" JSON error.
		log.DefaultLogger.FromContext(ctx).Warn("dataservice returned a non-JSON response",
			"status", statusCode, "body", bodySnippet(bodyBytes))
		return nil, buildNonJSONError(statusCode)
	}

	if response[RESP_CCODE] == jkql.ERROR {
		return nil, &queryError{buildCustomErrorMessage(fmt.Sprint(response[RESP_ERROR]))}
	}

	// A JSON body without the error envelope can still ride an HTTP error status — a gateway
	// or proxy answering for the dataservice. Don't let it pass as an empty result.
	if statusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("meshIQ Platform returned HTTP %d", statusCode)
	}

	return response, nil
}

// buildNonJSONError produces a readable error when the dataservice returns something other than
// the expected JSON result set (wrong Service URL, authentication redirect, reverse-proxy page…).
func buildNonJSONError(statusCode int) error {
	return fmt.Errorf("meshIQ Platform returned a non-JSON response (HTTP %d) — check the Service URL and access token", statusCode)
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
// content) can inject extra jk_* parameters. The access token is deliberately not a URL
// parameter — see HDR_API_KEY in protocol.go.
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

	return params
}

// maxResponseBodyBytes caps how much of a response body doRequest will read. Query results are
// bounded by jk_maxrows, but metadata calls send no row cap, and any request could hit a proxy
// serving something huge — unbounded io.ReadAll would buffer it all before parsing even starts.
const maxResponseBodyBytes = 100 * 1024 * 1024 // 100 MB

// errResponseTooLarge is returned when a response body exceeds maxResponseBodyBytes.
var errResponseTooLarge = fmt.Errorf("meshIQ Platform response exceeded the %d MB limit", maxResponseBodyBytes/(1024*1024))

// doGetRequest performs a GET bound to ctx (so panel cancels and alert deadlines propagate).
// apiKey, when non-empty, is sent as the X-API-Key header (the dataservice access token).
func doGetRequest(ctx context.Context, httpClient *http.Client, requestUrl string, apiKey string) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, 0, err
	}
	return doRequest(ctx, httpClient, request, apiKey)
}

// doFormPostRequest performs a POST bound to ctx, with formValues url-encoded into the request
// body as application/x-www-form-urlencoded — the same param string a GET would carry in its
// query string, without its URL-length limits (see handleSuggestions). apiKey, when non-empty,
// is sent as the X-API-Key header.
func doFormPostRequest(ctx context.Context, httpClient *http.Client, requestUrl string, formValues url.Values, apiKey string) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestUrl, strings.NewReader(formValues.Encode()))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return doRequest(ctx, httpClient, request, apiKey)
}

// doRequest sends request with the X-API-Key header attached (when apiKey is non-empty) and
// reads the response body up to maxResponseBodyBytes.
func doRequest(ctx context.Context, httpClient *http.Client, request *http.Request, apiKey string) ([]byte, int, error) {
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

	bodyBytes, err := readLimited(response.Body, maxResponseBodyBytes)
	return bodyBytes, response.StatusCode, err
}

// readLimited reads all of body, up to limit bytes, and fails with errResponseTooLarge if there
// was more. A separate function (limit as a parameter) so a test can pin the boundary behavior
// without transferring maxResponseBodyBytes' worth of data.
func readLimited(body io.Reader, limit int64) ([]byte, error) {
	// Read one byte past the limit so an exactly-at-the-limit body doesn't get misreported as
	// oversized, then check the actual count.
	bodyBytes, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(bodyBytes)) > limit {
		return nil, errResponseTooLarge
	}
	return bodyBytes, nil
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
