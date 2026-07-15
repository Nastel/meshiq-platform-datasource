package jkql

import (
	"encoding/json"
	"strings"
	"testing"
)

// parseRS unmarshals a raw result-set JSON string into the map BuildDataModel consumes. It
// decodes with UseNumber, exactly like the plugin's parseServiceResponse, so tests exercise
// the same value shapes (json.Number) production sees.
func parseRS(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&m); err != nil {
		t.Fatalf("unmarshal result set: %v", err)
	}
	return m
}

func TestBuildDataModel_ScalarColumns(t *testing.T) {
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["Severity", "EventCount"],
		"coltype": {"Severity": "STRING", "EventCount": "INTEGER"},
		"collabel": {"Severity": "Severity", "EventCount": "Event Count"},
		"rows": [
			{"Severity": "INFO",  "EventCount": 5},
			{"Severity": "ERROR", "EventCount": 2}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 2 {
		t.Fatalf("want 2 columns, got %d: %v", len(m.Headers), m.Headers)
	}
	if m.Label["EventCount"] != "Event Count" {
		t.Errorf("EventCount label = %q, want %q", m.Label["EventCount"], "Event Count")
	}
	if m.DataTypes["Severity"] != STRING || m.DataTypes["EventCount"] != INTEGER {
		t.Errorf("data types = %v", m.DataTypes)
	}
	if m.Rows[0]["Severity"] != "INFO" {
		t.Errorf("row 0 Severity = %v, want INFO", m.Rows[0]["Severity"])
	}
	if m.RowCount != 2 || m.TotalRowCount != 2 {
		t.Errorf("RowCount/TotalRowCount = %d/%d, want 2/2", m.RowCount, m.TotalRowCount)
	}
}

func TestBuildDataModel_MissingLabelFallsBackToHeader(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Severity"],
		"coltype": {"Severity": "STRING"},
		"collabel": {},
		"rows": [ {"Severity": "INFO"} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if got := m.Label["Severity"]; got != "Severity" {
		t.Errorf("label = %q, want the header used as a fallback", got)
	}
}

func TestBuildDataModel_ScoreColumnSkipped(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Severity", "Score"],
		"coltype": {"Severity": "STRING", "Score": "DECIMAL"},
		"collabel": {"Severity": "Severity", "Score": "Score"},
		"rows": [ {"Severity": "INFO", "Score": 1.0} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 1 || m.Headers[0] != "Severity" {
		t.Errorf("headers = %v, want just [Severity] (Score is a Solr internal field)", m.Headers)
	}
}

func TestBuildDataModel_ArrayColumn(t *testing.T) {
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["Tags"],
		"coltype": {"Tags": "STRING[]"},
		"collabel": {"Tags": "Tags"},
		"rows": [
			{"Tags": ["a", "b"]},
			{"Tags": null}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if m.DataTypes["Tags"] != STRING_ARR {
		t.Fatalf("data type = %v, want %v", m.DataTypes["Tags"], STRING_ARR)
	}
	row0, ok := m.Rows[0]["Tags"].([]interface{})
	if !ok || len(row0) != 2 {
		t.Errorf("row 0 Tags = %v, want a 2-element slice", m.Rows[0]["Tags"])
	}
	row1, ok := m.Rows[1]["Tags"].([]interface{})
	if !ok || len(row1) != 0 {
		t.Errorf("row 1 Tags = %v, want an empty slice (null normalizes to empty)", m.Rows[1]["Tags"])
	}
}

func TestBuildDataModel_NoRows(t *testing.T) {
	raw := `{"row-count": 0, "total-row-count": 0, "status": "SUCCESS", "colhdr": [], "coltype": {}, "collabel": {}, "rows": []}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 0 || len(m.Rows) != 0 {
		t.Errorf("empty result set should yield no headers/rows, got %v / %v", m.Headers, m.Rows)
	}
}
