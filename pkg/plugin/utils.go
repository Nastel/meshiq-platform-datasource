package plugin

import (
	"context"
	"strings"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// logParseIssues logs, once per result set, any wire-shape violations the converter recorded in
// model.Issues. The line carries the executed query and its date range, so the exact result set
// can be requested again to reproduce the issue.
func logParseIssues(ctx context.Context, queryModel QueryModel, model jkql.DataModel) {
	issues := model.Issues.List()
	if len(issues) == 0 {
		return
	}
	log.DefaultLogger.FromContext(ctx).Warn("result set had unparseable values; they are shown as empty",
		"issues", strings.Join(issues, "; "),
		"query", queryModel.JKQL,
		"date", queryModel.Date,
	)
}

// RemoveEndingSlash trims a single trailing slash from value.
func RemoveEndingSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return strings.TrimSuffix(value, "/")
	}
	return value
}
