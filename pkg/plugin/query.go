package plugin

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// QueryModel is the per-query model sent from the frontend query editor.
type QueryModel struct {
	JKQL         string `json:"jkql"`
	Locale       string `json:"locale"`
	Timezone     string `json:"timezone"`
	RepositoryID string `json:"repositoryID"`
	MaxRows      int    `json:"maxRows"`
	// Format selects the frame shape: jkql.FormatTable (default) or jkql.FormatTimeSeries.
	Format string `json:"format"`
	Date   string `json:"-"`
	// DebugRawResponse is set by the frontend only for Explore-originated queries (see
	// DataSource.query() in datasource.ts); it asks attachRawResponse to carry the dataservice's
	// raw response on the frame, viewable in the Query Inspector's JSON tab.
	DebugRawResponse bool `json:"debugRawResponse"`
}

// BuildQueryModel unmarshals a Grafana query and derives the jKQL date range.
func BuildQueryModel(query backend.DataQuery) (*QueryModel, error) {
	var queryModel QueryModel
	if err := json.Unmarshal(query.JSON, &queryModel); err != nil {
		return nil, err
	}

	queryModel.Date = buildDate(query.TimeRange)
	return &queryModel, nil
}

// buildDate converts a Grafana time range into the jKQL "<from> TO <to>" range
// expressed in microseconds, keeping sub-second precision.
func buildDate(timeRange backend.TimeRange) string {
	from := timeRange.From.UnixMicro()
	to := timeRange.To.UnixMicro()

	return fmt.Sprint(from, " TO ", to)
}

// BuildParamsQueryModel builds the jKQL used by the health check to read server parameters.
func BuildParamsQueryModel() QueryModel {
	return QueryModel{JKQL: "Get Params"}
}

// BuildRepositoriesQueryModel builds the jKQL used to list repositories.
func BuildRepositoriesQueryModel() QueryModel {
	return QueryModel{JKQL: "Get Repository Fields RepositoryID, RepositoryName, OrganizationName"}
}

// BuildFunctionsQueryModel builds the jKQL used to list the server's functions. The result set's
// Name column holds the function names and Type is Aggregate / Analytic / Scalar.
func BuildFunctionsQueryModel() QueryModel {
	return QueryModel{JKQL: "get functions"}
}

// BuildEnumValuesQueryModel builds the jKQL used to list the complete value set of a built-in enum
// field ("GET ENUMERATION FOR <field>"). The result set has an ID column (the ordinal) and a Name
// column, one row per possible enum value — used to build a full, gap-free enum Text table.
func BuildEnumValuesQueryModel(field string) QueryModel {
	return QueryModel{JKQL: "GET ENUMERATION FOR " + field}
}

// BuildItemsQueryModel builds the jKQL used to list item types (the "tables"). The result set's
// ItemName column holds the queryable item type names (Log, Event, Snapshot, …).
func BuildItemsQueryModel() QueryModel {
	return QueryModel{JKQL: "get items"}
}

// BuildFieldsQueryModel builds the jKQL used to list the fields of a single item type. Unlike a
// bare "get fields" (which lists only the static/built-in fields), "get fields for <itemType>"
// returns the static AND custom (Properties-derived) fields for that type — the custom ones are
// flagged via each field's Properties.isCustom.
func BuildFieldsQueryModel(itemType string) QueryModel {
	return QueryModel{JKQL: "get fields for " + itemType}
}
