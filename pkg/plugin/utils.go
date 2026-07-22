package plugin

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// logParseIssues logs, once per result set, any wire-shape violations the converter recorded in
// model.Issues. The line carries the executed query, its date range, and the repository, so the
// exact result set can be requested again to reproduce the issue.
func logParseIssues(ctx context.Context, queryModel QueryModel, model jkql.DataModel) {
	issues := model.Issues.List()
	if len(issues) == 0 {
		return
	}
	log.DefaultLogger.FromContext(ctx).Warn("result set had unparseable values; they are shown as empty",
		"issues", strings.Join(issues, "; "),
		"query", queryModel.JKQL,
		"date", queryModel.Date,
		"repo", queryModel.RepositoryID,
	)
}

// maxRawResponseBytes caps how much of the dataservice's raw response rides along on
// frame.Meta.Custom, so one huge result (e.g. a wide list() array) can't bloat every query
// response sent back to the browser.
const maxRawResponseBytes = 64 * 1024

// attachRawResponse stashes the dataservice's raw result-set JSON on the frame, viewable in
// Grafana's Query Inspector under the JSON tab's "DataFrame JSON (from Query)" view — the only
// place Meta.Custom actually renders in the UI. Truncated (with a note) past
// maxRawResponseBytes; silently skipped if it doesn't marshal (should not happen, response is
// already-parsed JSON). Called only when QueryModel.DebugRawResponse is set (Explore-originated
// queries — see datasource.ts's query()), so ordinary dashboard/alerting queries don't carry the
// extra payload back to the browser.
func attachRawResponse(frame *data.Frame, response ServiceResponse) {
	raw, err := json.Marshal(response)
	if err != nil {
		return
	}
	if frame.Meta == nil {
		frame.Meta = &data.FrameMeta{}
	}
	if len(raw) <= maxRawResponseBytes {
		frame.Meta.Custom = map[string]interface{}{"rawResponse": json.RawMessage(raw)}
		return
	}
	frame.Meta.Custom = map[string]interface{}{
		"rawResponse":       json.RawMessage(raw[:maxRawResponseBytes]),
		"rawResponseNotice": "truncated: response exceeded 64KB",
	}
}

// RemoveEndingSlash trims a single trailing slash from value.
func RemoveEndingSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return strings.TrimSuffix(value, "/")
	}
	return value
}
