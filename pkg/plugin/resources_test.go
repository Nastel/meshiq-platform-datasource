package plugin

import (
	"encoding/json"
	"testing"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
)

func parseResultSet(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal result set: %v", err)
	}
	return m
}

// TestFieldsExtraction mirrors handleFields: it verifies that FieldName/DataType are read and that
// the custom flag is taken from the exploded Properties('isCustom') column (jkql.BuildDataModel
// explodes the Properties MAP, so the flag is not a raw-map value on the row).
func TestFieldsExtraction(t *testing.T) {
	const raw = `{
		"row-count": 2, "total-row-count": 2,
		"item-type": "Field", "status": "SUCCESS",
		"colhdr": ["FieldName", "DataType", "Properties"],
		"coltype": {"FieldName": "STRING", "DataType": "ENUM", "Properties": "MAP"},
		"collabel": {"FieldName": "FieldName", "DataType": "DataType", "Properties": "Properties"},
		"rows": [
			{"FieldName": "Severity", "DataType": "7#ENUM",
			 "Properties": {"isCustom": false}, "Properties:_ValueTypes": {"isCustom": "BOOLEAN"}},
			{"FieldName": "Region", "DataType": "0#STRING",
			 "Properties": {"isCustom": true}, "Properties:_ValueTypes": {"isCustom": "BOOLEAN"}}
		]
	}`

	model := jkql.BuildDataModel(parseResultSet(t, raw), nil)

	customHeader := findHeaderByName(model, "Properties('isCustom')")
	if customHeader == "" {
		t.Fatal("expected an exploded Properties('isCustom') column")
	}

	type got struct {
		name, dataType string
		custom         bool
	}
	var results []got
	for _, row := range model.Rows {
		name, ok := grafanaString(model, row, jkql.FIELD_NAME)
		if !ok {
			continue
		}
		dataType, _ := grafanaString(model, row, jkql.DATA_TYPE)
		custom, _ := row[customHeader].(bool)
		results = append(results, got{name, dataType, custom})
	}

	want := []got{
		{"Severity", "ENUM", false},
		{"Region", "STRING", true},
	}
	if len(results) != len(want) {
		t.Fatalf("got %d fields, want %d: %+v", len(results), len(want), results)
	}
	for i, w := range want {
		if results[i] != w {
			t.Errorf("field %d = %+v, want %+v", i, results[i], w)
		}
	}
}

// TestTablesExtraction verifies /tables reads the ItemName column from a "get items" result set,
// including when ItemType is a LABELSET.
func TestTablesExtraction(t *testing.T) {
	const raw = `{
		"row-count": 3, "total-row-count": 3,
		"item-type": "Item", "status": "SUCCESS",
		"colhdr": ["ItemName", "ItemType"],
		"coltype": {"ItemName": "STRING", "ItemType": "LABELSET"},
		"collabel": {"ItemName": "ItemName", "ItemType": "ItemType"},
		"rows": [
			{"ItemName": "Log", "ItemType": "LOG"},
			{"ItemName": "Event", "ItemType": "EVENT"},
			{"ItemName": "Snapshot", "ItemType": "SNAPSHOT"}
		]
	}`

	model := jkql.BuildDataModel(parseResultSet(t, raw), nil)
	names := collectColumnStrings(model, jkql.ITEM_NAME)

	want := []string{"Log", "Event", "Snapshot"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("item %d = %q, want %q", i, names[i], w)
		}
	}
}
