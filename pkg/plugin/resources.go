package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// newResourceHandler builds the CallResource handler exposing the frontend's non-query endpoints:
//
//	GET /repositories -> ["<name>$<org>", …]     (jKQL: Get Repository Fields …)
//	GET /suggestions  -> autocomplete proxy      (see completion.go)
func (d *Datasource) newResourceHandler() backend.CallResourceHandler {
	mux := http.NewServeMux()
	mux.HandleFunc("/repositories", d.handleRepositories)
	mux.HandleFunc("/suggestions", d.handleSuggestions)
	return httpadapter.New(mux)
}

// handleRepositories lists the accessible repositories as "<RepositoryName>$<OrganizationName>"
// identifier strings (the frontend splits on "$" to group by organization).
func (d *Datasource) handleRepositories(w http.ResponseWriter, r *http.Request) {
	pCtx := backend.PluginConfigFromContext(r.Context())
	options, err := BuildMeshIqDataSourceOptions(pCtx.DataSourceInstanceSettings)
	if err != nil {
		writeResourceError(r, w, http.StatusBadRequest, err)
		return
	}

	// Listing repositories is inherently cross-repository, so the query carries no repository
	// of its own.
	queryModel := BuildRepositoriesQueryModel()
	result, err := queryDataService(r.Context(), d.httpClient, queryModel, *options)
	if err != nil {
		writeResourceError(r, w, http.StatusBadGateway, err)
		return
	}

	model := jkql.BuildDataModel(result, nil)
	logParseIssues(r.Context(), queryModel, model)

	writeResourceJSON(r, w, collectColumnStrings(model, jkql.REPO_ID))
}

// grafanaString reads a single column value from a row and normalizes it to a string via the
// column's data type (so ENUM -> name, STRING -> text). The bool reports whether the column
// exists and the value is non-nil; a non-string value is stringified, not rejected.
func grafanaString(model jkql.DataModel, row map[string]interface{}, header string) (string, bool) {
	dataType, ok := model.DataTypes[header]
	if !ok {
		return "", false
	}
	value := jkql.ConvertToGrafanaValue(row[header], dataType)
	if value == nil {
		return "", false
	}
	s, ok := value.(string)
	if !ok {
		return fmt.Sprint(value), true
	}
	return s, true
}

// collectColumnStrings returns the non-empty string values of a single column, in row order.
func collectColumnStrings(model jkql.DataModel, header string) []string {
	values := make([]string, 0, len(model.Rows))
	for _, row := range model.Rows {
		if s, ok := grafanaString(model, row, header); ok && s != "" {
			values = append(values, s)
		}
	}
	return values
}

func writeResourceJSON(r *http.Request, w http.ResponseWriter, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		writeResourceError(r, w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		log.DefaultLogger.FromContext(r.Context()).Error("failed to write resource response", "path", r.URL.Path, "error", err)
	}
}

// writeResourceError answers a resource request with a JSON error and logs it. The frontend
// degrades quietly on these (an empty repository dropdown, no suggestions), so this log line is
// the only place a failing /repositories or /suggestions call becomes visible.
func writeResourceError(r *http.Request, w http.ResponseWriter, status int, err error) {
	log.DefaultLogger.FromContext(r.Context()).Warn("resource request failed",
		"path", r.URL.Path, "status", status, "error", err)
	writeResourceErrorBody(r, w, status, err)
}

// writeResourceErrorQuiet answers a resource request with a JSON error, without logging it.
// Reserved for failures that are expected and frequent by design rather than actionable — a jKQL
// autocomplete request rejected because the query is momentarily invalid mid-typing happens on
// nearly every keystroke and would otherwise flood the server log with noise, for a feature that
// already degrades silently (no suggestions) in the editor.
func writeResourceErrorQuiet(r *http.Request, w http.ResponseWriter, status int, err error) {
	writeResourceErrorBody(r, w, status, err)
}

func writeResourceErrorBody(r *http.Request, w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	if _, werr := w.Write(body); werr != nil {
		log.DefaultLogger.FromContext(r.Context()).Error("failed to write resource error", "path", r.URL.Path, "error", werr)
	}
}
