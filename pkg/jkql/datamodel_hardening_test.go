package jkql

import (
	"strings"
	"testing"
)

func TestBuildDataModel_RangeExplodesToBeginEnd(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Duration"],
		"coltype": {"Duration": "RANGE(INTEGER)"},
		"collabel": {"Duration": "Duration"},
		"rows": [ {"Duration": {"begin": 10, "end": 20}} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 2 {
		t.Fatalf("want 2 columns (begin/end), got %d: %v", len(m.Headers), m.Headers)
	}
	if m.DataTypes["Duration_begin"] != INTEGER || m.DataTypes["Duration_end"] != INTEGER {
		t.Errorf("range element type = %v", m.DataTypes)
	}
	begin, ok := ToInt64(m.Rows[0]["Duration_begin"])
	if !ok || begin != 10 {
		t.Errorf("Duration_begin = %v, want 10", m.Rows[0]["Duration_begin"])
	}
	end, ok := ToInt64(m.Rows[0]["Duration_end"])
	if !ok || end != 20 {
		t.Errorf("Duration_end = %v, want 20", m.Rows[0]["Duration_end"])
	}
}

func TestBuildDataModel_GenericRangeFallsBackToString(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Bucket"],
		"coltype": {"Bucket": "RANGE"},
		"collabel": {"Bucket": "Bucket"},
		"rows": [ {"Bucket": {"begin": "a", "end": "z"}} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if m.DataTypes["Bucket_begin"] != STRING || m.DataTypes["Bucket_end"] != STRING {
		t.Errorf("generic RANGE element type = %v, want STRING endpoints", m.DataTypes)
	}
}

func TestBuildDataModel_MalformedRangeRecordsIssueAndNulls(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Duration"],
		"coltype": {"Duration": "RANGE(INTEGER)"},
		"collabel": {"Duration": "Duration"},
		"rows": [ {"Duration": "not-a-range"} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if m.Rows[0]["Duration_begin"] != nil || m.Rows[0]["Duration_end"] != nil {
		t.Errorf("malformed range should null both bounds, got begin=%v end=%v",
			m.Rows[0]["Duration_begin"], m.Rows[0]["Duration_end"])
	}
	if len(m.Issues.List()) == 0 {
		t.Error("malformed range should record a parse issue")
	}
}

func TestBuildDataModel_MalformedRowRecordsIssueAndNulls(t *testing.T) {
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["Name"],
		"coltype": {"Name": "STRING"},
		"collabel": {"Name": "Name"},
		"rows": [ {"Name": "ok"}, "not-an-object" ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if m.Rows[0]["Name"] != "ok" {
		t.Errorf("row 0 Name = %v, want ok", m.Rows[0]["Name"])
	}
	if m.Rows[1]["Name"] != nil {
		t.Errorf("malformed row should null its value, got %v", m.Rows[1]["Name"])
	}
	if len(m.Issues.List()) == 0 {
		t.Error("a malformed row should record a parse issue")
	}
}

func TestBuildDataModel_MissingColtypeRecordsIssueAndSkipsColumns(t *testing.T) {
	raw := `{"row-count": 1, "total-row-count": 1, "status": "SUCCESS", "colhdr": ["Name"], "collabel": {"Name": "Name"}, "rows": [ {"Name": "x"} ]}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 0 {
		t.Errorf("without coltype no column can be built, got %v", m.Headers)
	}
	if len(m.Issues.List()) == 0 {
		t.Error("missing coltype should record a parse issue")
	}
}

func TestBuildDataModel_MissingRowsArrayRecordsIssue(t *testing.T) {
	raw := `{"row-count": 2, "total-row-count": 2, "status": "SUCCESS", "colhdr": ["Name"], "coltype": {"Name": "STRING"}, "collabel": {"Name": "Name"}}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Issues.List()) == 0 {
		t.Error("a result set that claims rows but has none should record a parse issue")
	}
}

func TestBuildDataModel_NegativeRowCountClampedToZero(t *testing.T) {
	raw := `{"row-count": -1, "total-row-count": 0, "status": "SUCCESS", "colhdr": ["Name"], "coltype": {"Name": "STRING"}, "collabel": {"Name": "Name"}, "rows": []}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if m.RowCount != 0 {
		t.Errorf("RowCount = %d, want 0", m.RowCount)
	}
	if len(m.Issues.List()) == 0 {
		t.Error("a negative row-count should record a parse issue")
	}
}

func TestBuildDataModel_RowCountSmallerThanRowsArrayGrowsToMatch(t *testing.T) {
	raw := `{"row-count": 1, "total-row-count": 2, "status": "SUCCESS", "colhdr": ["Name"], "coltype": {"Name": "STRING"}, "collabel": {"Name": "Name"}, "rows": [ {"Name": "a"}, {"Name": "b"} ]}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if m.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2 (the rows array wins)", m.RowCount)
	}
	if len(m.Issues.List()) == 0 {
		t.Error("a row-count smaller than the rows array should record a parse issue")
	}
}

// TestBuildDataModel_HugeRowCountClampedNotAllocated guards against a huge declared row-count
// (garbage, or an ID column misread as a count) sizing Rows off that number directly — which
// would attempt a multi-terabyte allocation and crash the process before a single row is read.
func TestBuildDataModel_HugeRowCountClampedNotAllocated(t *testing.T) {
	raw := `{"row-count": 1000000000000, "total-row-count": 0, "status": "SUCCESS", "colhdr": ["Name"], "coltype": {"Name": "STRING"}, "collabel": {"Name": "Name"}, "rows": [ {"Name": "a"} ]}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if m.RowCount > 1+maxRowCountOverhead {
		t.Errorf("RowCount = %d, want at most %d (clamped)", m.RowCount, 1+maxRowCountOverhead)
	}
	if len(m.Issues.List()) == 0 {
		t.Error("a row-count far larger than the rows array should record a parse issue")
	}
}

// TestBuildDataModel_MixedMapIntDecimalKeyMergesIntoOneColumn exercises mergeIntColumnsIntoDecimal
// through its real producer: a mixed-map key (Properties, via explodeMixedMap) that holds an
// INTEGER on one row and a DECIMAL on another explodes into two type-prefixed columns
// (I:Properties('Quota'), D:Properties('Quota')); the merge collapses them back into one
// DECIMAL-typed column so the field reads as one consistent numeric column instead of two.
// TestBuildDataModel_MixedMapIntDecimalStringKeyKeepsDecimalLabelSuffix exercises
// mergeIntColumnsIntoDecimal when a mixed-map key holds three types across rows (INTEGER,
// DECIMAL, and STRING): the int/decimal pair still merges into one DECIMAL column, but its
// label must keep the " (Decimal)" suffix, since the STRING sibling column is still around and
// the suffix is still disambiguating between them.
func TestBuildDataModel_MixedMapIntDecimalStringKeyKeepsDecimalLabelSuffix(t *testing.T) {
	raw := `{
		"row-count": 3, "total-row-count": 3, "status": "SUCCESS",
		"colhdr": ["Properties('Quota')"],
		"coltype": {"Properties('Quota')": "MAP"},
		"collabel": {"Properties('Quota')": "Quota"},
		"rows": [
			{"Properties('Quota')": {"Quota": 5}, "Properties('Quota'):_ValueTypes": {"Quota": "INTEGER"}},
			{"Properties('Quota')": {"Quota": 2.5}, "Properties('Quota'):_ValueTypes": {"Quota": "DECIMAL"}},
			{"Properties('Quota')": {"Quota": "unlimited"}, "Properties('Quota'):_ValueTypes": {"Quota": "STRING"}}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 2 {
		t.Fatalf("want the int/decimal pair merged, leaving 2 columns (decimal, string), got %d: %v", len(m.Headers), m.Headers)
	}

	var decimalHeader, stringHeader string
	for _, h := range m.Headers {
		switch m.DataTypes[h] {
		case DECIMAL:
			decimalHeader = h
		case STRING:
			stringHeader = h
		}
	}
	if decimalHeader == "" || stringHeader == "" {
		t.Fatalf("want one DECIMAL and one STRING column, got types %v", m.DataTypes)
	}
	if !strings.HasSuffix(m.Label[decimalHeader], " (Decimal)") {
		t.Errorf("merged decimal column label = %q, want it to keep the (Decimal) suffix while a STRING sibling remains", m.Label[decimalHeader])
	}
	if !strings.HasSuffix(m.Label[stringHeader], " (String)") {
		t.Errorf("string column label = %q, want it to keep the (String) suffix", m.Label[stringHeader])
	}
}

func TestBuildDataModel_MixedMapIntDecimalKeyMergesIntoOneColumn(t *testing.T) {
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["Properties('Quota')"],
		"coltype": {"Properties('Quota')": "MAP"},
		"collabel": {"Properties('Quota')": "Quota"},
		"rows": [
			{"Properties('Quota')": {"Quota": 5}, "Properties('Quota'):_ValueTypes": {"Quota": "INTEGER"}},
			{"Properties('Quota')": {"Quota": 2.5}, "Properties('Quota'):_ValueTypes": {"Quota": "DECIMAL"}}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 1 {
		t.Fatalf("want the int/decimal pair merged into 1 column, got %d: %v", len(m.Headers), m.Headers)
	}
	header := m.Headers[0]
	if m.DataTypes[header] != DECIMAL {
		t.Errorf("merged column type = %v, want DECIMAL", m.DataTypes[header])
	}
	f0, ok := ToFloat64(m.Rows[0][header])
	if !ok || f0 != 5 {
		t.Errorf("row 0 merged value = %v, want the integer 5 filled in", m.Rows[0][header])
	}
	f1, ok := ToFloat64(m.Rows[1][header])
	if !ok || f1 != 2.5 {
		t.Errorf("row 1 merged value = %v, want 2.5 (untouched)", m.Rows[1][header])
	}
}

// TestBuildDataModel_MergeDropsIntHeaderFromNames pins that merging an I:-prefixed column into its
// D:-prefixed twin also removes the merged I: header from model.Names — the merge drops the column
// from Headers, so a leftover Names entry would be dead metadata referencing a column that no
// longer exists.
func TestBuildDataModel_MergeDropsIntHeaderFromNames(t *testing.T) {
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["Properties('Quota')"],
		"coltype": {"Properties('Quota')": "MAP"},
		"collabel": {"Properties('Quota')": "Quota"},
		"rows": [
			{"Properties('Quota')": {"Quota": 5}, "Properties('Quota'):_ValueTypes": {"Quota": "INTEGER"}},
			{"Properties('Quota')": {"Quota": 2.5}, "Properties('Quota'):_ValueTypes": {"Quota": "DECIMAL"}}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	for header := range m.Names {
		if strings.HasPrefix(header, "I:") {
			t.Errorf("Names still holds the merged int header %q, want it deleted", header)
		}
	}
}
