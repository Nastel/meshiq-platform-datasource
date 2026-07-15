package jkql

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConvertToGrafanaValue_Scalars(t *testing.T) {
	cases := []struct {
		name     string
		value    interface{}
		dataType string
		want     interface{}
	}{
		{"nil stays nil", nil, STRING, nil},
		{"integer float->int64", float64(42), INTEGER, int64(42)},
		{"decimal", float64(1.5), DECIMAL, 1.5},
		{"string", "hi", STRING, "hi"},
		{"clob is a string", "some text", CLOB, "some text"},
		{"enum -> name from ordinal#name", "3#INFO", ENUM, "INFO"},
		{"boolean", true, BOOLEAN, true},
		{"timeinterval float->int64 micros", float64(1500), TIMEINTERVAL, int64(1500)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ConvertToGrafanaValue(c.value, c.dataType)
			if got != c.want {
				t.Errorf("ConvertToGrafanaValue(%v, %s) = %#v, want %#v", c.value, c.dataType, got, c.want)
			}
		})
	}
}

func TestConvertToGrafanaValue_BigIntegerKeepsPrecision(t *testing.T) {
	// jKQL INTEGER is a Java long. Result sets are decoded with UseNumber, so values arrive
	// as json.Number and survive past 2^53, where float64 would round them.
	big := "9007199254740993" // 2^53 + 1
	got := ConvertToGrafanaValue(json.Number(big), INTEGER)
	if got != int64(9007199254740993) {
		t.Errorf("INTEGER %s -> %v, precision lost", big, got)
	}

	if got := ConvertToGrafanaValue(json.Number("1.5"), DECIMAL); got != 1.5 {
		t.Errorf("DECIMAL json.Number -> %v, want 1.5", got)
	}
}

func TestConvertToGrafanaValue_Timestamp(t *testing.T) {
	// jKQL timestamps are microseconds; conversion must keep the full microsecond precision so
	// events within the same second stay ordered.
	got := ConvertToGrafanaValue(float64(2_000_123), TIMESTAMP)
	ts, ok := got.(time.Time)
	if !ok {
		t.Fatalf("expected a time.Time, got %#v", got)
	}
	if ts.UnixMicro() != 2_000_123 {
		t.Errorf("timestamp = %v (unix micro %d), want 2000123", ts, ts.UnixMicro())
	}
}

func TestConvertToGrafanaValue_Array(t *testing.T) {
	arr := ConvertToGrafanaValue([]interface{}{"a", "b"}, STRING_ARR)
	if raw, ok := arr.(json.RawMessage); !ok || string(raw) != `["a","b"]` {
		t.Errorf("STRING_ARR -> %q (%T), want [\"a\",\"b\"]", arr, arr)
	}
}

func TestBuildDataFrame_FieldsMatchModel(t *testing.T) {
	raw := `{
		"row-count": 2, "total-row-count": 5, "status": "SUCCESS",
		"colhdr": ["Severity", "EventCount"],
		"coltype": {"Severity": "STRING", "EventCount": "INTEGER"},
		"collabel": {"Severity": "Severity", "EventCount": "Event Count"},
		"rows": [
			{"Severity": "INFO",  "EventCount": 5},
			{"Severity": "ERROR", "EventCount": 2}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)
	frame := BuildDataFrame(m)

	if len(frame.Fields) != 2 {
		t.Fatalf("want 2 fields, got %d", len(frame.Fields))
	}
	if frame.Fields[0].Name != "Severity" || frame.Fields[1].Name != "Event Count" {
		t.Errorf("field names = %q, %q", frame.Fields[0].Name, frame.Fields[1].Name)
	}
	if frame.Fields[0].Len() != 2 {
		t.Errorf("field length = %d, want 2", frame.Fields[0].Len())
	}

	// Row statistics land in the query inspector via frame.Meta.Stats.
	if frame.Meta == nil || len(frame.Meta.Stats) != 2 {
		t.Fatalf("want 2 query stats, got %v", frame.Meta)
	}
	if frame.Meta.Stats[0].Value != 2 || frame.Meta.Stats[1].Value != 5 {
		t.Errorf("stats = %+v, want rows returned=2, total rows matched=5", frame.Meta.Stats)
	}
}

func TestFinalizeFrame_SetsExecutedQuery(t *testing.T) {
	m := BuildDataModel(parseRS(t, `{"row-count": 0, "total-row-count": 0, "status": "SUCCESS", "colhdr": [], "coltype": {}, "collabel": {}, "rows": []}`), nil)
	frame := BuildDataFrame(m)

	frame = FinalizeFrame(frame, "Get Events")
	if frame.Meta.ExecutedQueryString != "Get Events" {
		t.Errorf("ExecutedQueryString = %q, want %q", frame.Meta.ExecutedQueryString, "Get Events")
	}
}
