package plugin

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// QueryModel is the per-query model sent from the frontend query editor.
type QueryModel struct {
	JKQL     string `json:"jkql"`
	Locale   string `json:"locale"`
	Timezone string `json:"timezone"`
	MaxRows  int    `json:"maxRows"`
	Date     string `json:"-"`
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
