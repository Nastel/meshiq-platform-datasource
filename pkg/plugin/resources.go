package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// tableInfo is one item type ("table") returned by /tables.
type tableInfo struct {
	Name string `json:"name"`
}

// fieldInfo is one field of an item type returned by /fields. Custom is true for fields derived
// from the Properties map (as opposed to the static/built-in fields).
type fieldInfo struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Custom bool   `json:"custom"`
}

// newResourceHandler builds the CallResource handler exposing the frontend's non-query endpoints:
//
//	GET /repositories   -> ["<name>$<org>", …]    (jKQL: Get Repository Fields …)
//	GET /tables         -> [{name}]               (jKQL: get items)
//	GET /fields?table=X -> [{name,type,custom}]   (jKQL: get fields for X)
//	GET /suggestions    -> autocomplete proxy     (see completion.go)
func (d *Datasource) newResourceHandler() backend.CallResourceHandler {
	mux := http.NewServeMux()
	mux.HandleFunc("/repositories", d.handleRepositories)
	mux.HandleFunc("/tables", d.handleTables)
	mux.HandleFunc("/fields", d.handleFields)
	mux.HandleFunc("/suggestions", d.handleSuggestions)
	return httpadapter.New(mux)
}

// errInvalidOptions marks a metadata-query failure that happened before any dataservice call was
// made (options parsing/validation) — a config problem on this end, not a downstream outage.
var errInvalidOptions = errors.New("invalid datasource options")

// handleRepositories lists the accessible repositories as "<RepositoryName>$<OrganizationName>"
// identifier strings (the frontend splits on "$" to group by organization).
func (d *Datasource) handleRepositories(w http.ResponseWriter, r *http.Request) {
	// Listing repositories is inherently cross-repository, so no default-repo scoping here.
	model, err := d.runMetadataQuery(r, BuildRepositoriesQueryModel(), false)
	if err != nil {
		writeResourceError(r, w, metadataQueryErrorStatus(err), err)
		return
	}

	writeResourceJSON(r, w, collectColumnStrings(model, jkql.REPO_ID))
}

// metadataQueryErrorStatus classifies a runMetadataQuery error the same way the main query path
// does: an invalid local config is a client-side problem (400), a dataservice error envelope means
// the dataservice itself rejected the query (400), and everything else (transport failure) is the
// only case that's actually a gateway/downstream outage (502).
func metadataQueryErrorStatus(err error) int {
	if errors.Is(err, errInvalidOptions) {
		return http.StatusBadRequest
	}
	var qe *queryError
	if errors.As(err, &qe) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

// runMetadataQuery resolves the datasource options for the request and runs a jKQL query,
// returning the parsed, exploded jkql.DataModel. useDefaultRepo scopes the query to the
// datasource's default repository — metadata like custom fields is repository data, so without
// it the token's implicit default repository would answer instead of the configured one.
func (d *Datasource) runMetadataQuery(r *http.Request, queryModel QueryModel, useDefaultRepo bool) (jkql.DataModel, error) {
	pCtx := backend.PluginConfigFromContext(r.Context())
	options, err := BuildMeshIqDataSourceOptions(pCtx.DataSourceInstanceSettings)
	if err != nil {
		return jkql.DataModel{}, fmt.Errorf("%w: %w", errInvalidOptions, err)
	}

	if useDefaultRepo && queryModel.RepositoryID == "" {
		queryModel.RepositoryID = options.RepositoryID
	}

	result, err := queryDataService(r.Context(), d.httpClient, queryModel, *options)
	if err != nil {
		return jkql.DataModel{}, err
	}

	model := jkql.BuildDataModel(result, nil)
	logParseIssues(r.Context(), queryModel, model)
	return model, nil
}

// handleTables lists the queryable item types ("get items").
func (d *Datasource) handleTables(w http.ResponseWriter, r *http.Request) {
	model, err := d.runMetadataQuery(r, BuildItemsQueryModel(), true)
	if err != nil {
		writeResourceError(r, w, metadataQueryErrorStatus(err), err)
		return
	}

	tables := make([]tableInfo, 0, model.RowCount)
	for _, name := range collectColumnStrings(model, jkql.ITEM_NAME) {
		tables = append(tables, tableInfo{Name: name})
	}

	writeResourceJSON(r, w, tables)
}

// handleFields lists the static and custom fields of the item type named by the "table" query
// parameter ("get fields for <table>").
func (d *Datasource) handleFields(w http.ResponseWriter, r *http.Request) {
	itemType := r.URL.Query().Get("table")
	if !jkql.IdentifierRegExp.MatchString(itemType) {
		writeResourceError(r, w, http.StatusBadRequest, fmt.Errorf("missing or invalid 'table' parameter"))
		return
	}

	model, err := d.runMetadataQuery(r, BuildFieldsQueryModel(itemType), true)
	if err != nil {
		writeResourceError(r, w, metadataQueryErrorStatus(err), err)
		return
	}

	// jkql.BuildDataModel explodes the Properties MAP into per-key columns, so the isCustom flag
	// lives in the exploded boolean column keyed by the jKQL name Properties('isCustom').
	customHeader := findHeaderByName(model, "Properties('isCustom')")

	fields := make([]fieldInfo, 0, model.RowCount)
	for _, row := range model.Rows {
		name, ok := grafanaString(model, row, jkql.FIELD_NAME)
		if !ok || name == "" {
			continue
		}
		dataType, _ := grafanaString(model, row, jkql.DATA_TYPE)
		custom := false
		if customHeader != "" {
			custom, _ = row[customHeader].(bool)
		}
		fields = append(fields, fieldInfo{
			Name:   name,
			Type:   dataType,
			Custom: custom,
		})
	}

	writeResourceJSON(r, w, fields)
}

// findHeaderByName returns the (exploded) column header whose jKQL field name equals fieldName, or
// "" if none matches. jkql.BuildDataModel keys each column by its jKQL name in model.Names.
func findHeaderByName(model jkql.DataModel, fieldName string) string {
	for header, name := range model.Names {
		if name == fieldName {
			return header
		}
	}
	return ""
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

// grafanaInt reads a single column value from a row and normalizes it to an int via the column's
// data type. The bool reports whether the column exists and holds an integer/numeric value.
func grafanaInt(model jkql.DataModel, row map[string]interface{}, header string) (int, bool) {
	dataType, ok := model.DataTypes[header]
	if !ok {
		return 0, false
	}
	value := jkql.ConvertToGrafanaValue(row[header], dataType)
	switch n := value.(type) {
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
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
// degrades quietly on these (empty dropdowns, no suggestions), so this log line is the only place
// a failing /repositories, /tables, /fields or /suggestions call becomes visible.
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
