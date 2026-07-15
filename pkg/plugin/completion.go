package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// Autocomplete-service query-param names. Together with the /suggestions path and the response
// shape below they form the wire contract with the jKQL autocomplete service, so they must not
// be renamed.
const (
	REQ_POSITION RequestParameter = "jk_position"
)

// completionItem mirrors one entry returned by the jKQL autocomplete service. The JSON shape is the
// wire contract: { "label", "insertText"?, "kind", "deleteBackwards"? }. Fields are forwarded to the
// frontend unchanged; kind is the service's enum name (StatementType, ItemType, Keyword, Field, …).
type completionItem struct {
	Label           string `json:"label"`
	InsertText      string `json:"insertText,omitempty"`
	Kind            string `json:"kind"`
	DeleteBackwards *int   `json:"deleteBackwards,omitempty"`
}

// handleSuggestions proxies a jKQL completion request to the configured autocomplete service and
// returns its CompletionItem array unchanged. The wire contract with the service:
//
//	POST /suggestions, body (application/x-www-form-urlencoded): jk_query=<jkql>&jk_position=<caret>&jk_repo=<repo> ->
//	    [{label, insertText?, kind, deleteBackwards?}]
//
// POST (not GET) so a long jKQL query can't run into proxy/server URL-length limits; the
// service treats a form-encoded POST body the same as GET query params (confirmed against its
// AutoCompleteMsgHandler).
//
// When completion is disabled or unconfigured it returns an empty list (never an error), so the
// editor degrades quietly to no suggestions.
func (d *Datasource) handleSuggestions(w http.ResponseWriter, r *http.Request) {
	pCtx := backend.PluginConfigFromContext(r.Context())
	options, err := BuildMeshIqDataSourceOptions(pCtx.DataSourceInstanceSettings)
	if err != nil {
		writeResourceError(r, w, http.StatusBadRequest, err)
		return
	}

	if !options.EnableCompletion || options.CompletionServiceUrl == "" {
		writeResourceJSON(r, w, []completionItem{})
		return
	}

	requestUrl, formValues, err := buildSuggestionsRequest(*options, r.URL.Query())
	if err != nil {
		// A malformed caller parameter (e.g. a non-integer jk_position) — the request never left
		// the plugin, so this is the caller's mistake, not the completion service's.
		writeResourceError(r, w, http.StatusBadRequest, err)
		return
	}

	items, err := requestSuggestions(r.Context(), d.httpClient, requestUrl, formValues)
	if err != nil {
		// Typically the completion service rejecting a jKQL query that's momentarily invalid
		// mid-typing — expected on nearly every keystroke, not worth a server-log warning for a
		// feature that already degrades silently (no suggestions) in the editor.
		writeResourceErrorQuiet(r, w, http.StatusBadGateway, err)
		return
	}

	writeResourceJSON(r, w, items)
}

// requestSuggestions calls the autocomplete service and parses its CompletionItem array.
func requestSuggestions(ctx context.Context, httpClient *http.Client, requestUrl string, formValues url.Values) ([]completionItem, error) {
	bodyBytes, statusCode, err := doFormPostRequest(ctx, httpClient, requestUrl, formValues, "") // no token on this service
	if err != nil {
		return nil, err
	}
	if statusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("completion service returned HTTP %d: %s", statusCode, bodySnippet(bodyBytes))
	}

	var items []completionItem
	if err := json.Unmarshal(bodyBytes, &items); err != nil {
		return nil, buildNonJSONError(statusCode, bodyBytes)
	}
	return items, nil
}

// buildSuggestionsRequest builds the service's bare /suggestions URL and the form-encoded
// jk_query / jk_position / jk_repo values to send in the POST body, defaulting the repository to
// the datasource default when unset. Params are rebuilt with url.Values (never concatenated), so
// caller-supplied values cannot inject extra parameters into the outbound request; jk_position
// must be an integer.
func buildSuggestionsRequest(options MeshIqDataSourceOptions, query url.Values) (string, url.Values, error) {
	params := url.Values{}
	params.Set(REQ_QUERY, query.Get(REQ_QUERY))

	if position := query.Get(REQ_POSITION); position != "" {
		if _, err := strconv.Atoi(position); err != nil {
			return "", nil, fmt.Errorf("invalid %s %q: must be an integer", REQ_POSITION, position)
		}
		params.Set(REQ_POSITION, position)
	}

	repo := query.Get(REQ_REPO)
	if repo == "" {
		repo = options.RepositoryID
	}
	if repo != "" {
		params.Set(REQ_REPO, repo)
	}

	base := RemoveEndingSlash(options.CompletionServiceUrl)
	return base + "/suggestions", params, nil
}

// testCompletionService verifies the autocomplete service is reachable (used by CheckHealth when
// completion is enabled). It pings the service's /version endpoint.
func testCompletionService(ctx context.Context, httpClient *http.Client, options MeshIqDataSourceOptions) error {
	base := RemoveEndingSlash(options.CompletionServiceUrl)
	_, statusCode, err := doGetRequest(ctx, httpClient, base+"/version", "") // no token on this service
	if err != nil {
		return fmt.Errorf("completion service unreachable: %w", err)
	}
	if statusCode >= http.StatusBadRequest {
		return fmt.Errorf("completion service returned HTTP %d", statusCode)
	}
	return nil
}
