package jkql

import (
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func TestConvertToGrafanaValue_Labelset(t *testing.T) {
	if got := ConvertToGrafanaValue("Allow", LABELSET); got != "Allow" {
		t.Errorf("LABELSET -> %v, want the raw label", got)
	}
}

func TestConvertToGrafanaValue_StringUnwrapsSingleKeyMap(t *testing.T) {
	// A Properties('key') value can reach here still wrapped in its single-key map envelope
	// (e.g. via Coalesce(Properties('key'), ...)) when the header doesn't match the patterns
	// BuildDataModel unwraps upfront. Must extract the real value, not stringify the Go map.
	value := map[string]interface{}{"UserName": "Administrator"}
	if got := ConvertToGrafanaValue(value, STRING); got != "Administrator" {
		t.Errorf("STRING with single-key map envelope = %#v, want the unwrapped value", got)
	}
}

// TestConvertToGrafanaValue_StringSingleKeyMapWithNilValue pins that a single-key map envelope
// whose value is nil unwraps to nil, not fmt.Sprint's "<nil>" text.
func TestConvertToGrafanaValue_StringSingleKeyMapWithNilValue(t *testing.T) {
	value := map[string]interface{}{"UserName": nil}
	if got := ConvertToGrafanaValue(value, STRING); got != nil {
		t.Errorf("STRING with a nil single-key map value = %#v, want nil", got)
	}
}

// TestConvertToGrafanaValue_StringRendersContainersAsJSON pins that a STRING-typed value that
// unexpectedly arrives as a multi-key map or an array renders as JSON, not fmt.Sprint's Go syntax
// ("map[a:1 b:2]", "[a b]").
func TestConvertToGrafanaValue_StringRendersContainersAsJSON(t *testing.T) {
	mapValue := map[string]interface{}{"a": "1", "b": "2"}
	got := ConvertToGrafanaValue(mapValue, STRING)
	gotStr, ok := got.(string)
	if !ok {
		t.Fatalf("STRING with a multi-key map = %#v (%T), want a JSON string", got, got)
	}
	if strings.Contains(gotStr, "map[") {
		t.Errorf("STRING with a multi-key map = %q, leaked Go map syntax", gotStr)
	}

	arrValue := []interface{}{"a", "b"}
	got = ConvertToGrafanaValue(arrValue, STRING)
	gotStr, ok = got.(string)
	if !ok {
		t.Fatalf("STRING with an array value = %#v (%T), want a JSON string", got, got)
	}
	if gotStr == "[a b]" {
		t.Errorf("STRING with an array value = %q, leaked Go slice syntax", gotStr)
	}
}

func TestConvertToGrafanaValue_Variant(t *testing.T) {
	cases := []struct {
		name     string
		envelope map[string]interface{}
		want     interface{}
	}{
		{"integer collapses to decimal", map[string]interface{}{"data-type": "INTEGER", "value": float64(5)}, 5.0},
		{"decimal stays decimal", map[string]interface{}{"data-type": "DECIMAL", "value": 1.5}, 1.5},
		{"boolean keeps its real type", map[string]interface{}{"data-type": "BOOLEAN", "value": true}, true},
		{"unknown data-type falls back to string", map[string]interface{}{"data-type": "WEIRD", "value": "x"}, "x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ConvertToGrafanaValue(c.envelope, VARIANT)
			if got != c.want {
				t.Errorf("ConvertToGrafanaValue(%v, VARIANT) = %#v, want %#v", c.envelope, got, c.want)
			}
		})
	}
}

func TestBuildDataFrame_VariantColumnSplitsByType(t *testing.T) {
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["Payload"],
		"coltype": {"Payload": "VARIANT"},
		"collabel": {"Payload": "Payload"},
		"rows": [
			{"Payload": {"data-type": "INTEGER", "value": 5}},
			{"Payload": {"data-type": "STRING", "value": "hi"}}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)
	frame := BuildDataFrame(m, nil)

	// A VARIANT column mixing a number and a string splits into a DECIMAL sub-column and a
	// STRING sub-column, each keeping only the rows of its own type.
	if len(frame.Fields) != 2 {
		t.Fatalf("want 2 sub-columns (DECIMAL + STRING), got %d: %v", len(frame.Fields), frame.Fields)
	}
	var sawDecimal, sawString bool
	for _, f := range frame.Fields {
		switch f.Name {
		case "Payload (DECIMAL)":
			sawDecimal = true
		case "Payload (STRING)":
			sawString = true
		}
	}
	if !sawDecimal || !sawString {
		t.Errorf("field names = %v, want one DECIMAL and one STRING sub-column", []string{frame.Fields[0].Name, frame.Fields[1].Name})
	}
}

func TestBuildDataFrame_VariantBooleanKeepsOwnType(t *testing.T) {
	// A BOOLEAN variant value must render as a real bool, not collapse into the STRING
	// sub-column as the text "true"/"false".
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["Val"],
		"coltype": {"Val": "VARIANT"},
		"collabel": {"Val": "Val"},
		"rows": [
			{"Val": {"data-type": "BOOLEAN", "value": true}},
			{"Val": {"data-type": "STRING", "value": "hi"}}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)
	frame := BuildDataFrame(m, nil)

	fields := map[string]*data.Field{}
	for _, f := range frame.Fields {
		fields[f.Name] = f
	}
	boolCol, str := fields["Val (BOOLEAN)"], fields["Val (STRING)"]
	if boolCol == nil || str == nil {
		t.Fatalf("want sub-columns Val (BOOLEAN) and Val (STRING), got %v", frame.Fields)
	}
	if got := boolCol.At(0).(*bool); got == nil || *got != true {
		t.Errorf("boolean[0] = %v, want true", got)
	}
	if got := str.At(1).(*string); got == nil || *got != "hi" {
		t.Errorf("string[1] = %v, want hi", got)
	}
}

func TestBuildDataFrame_AllNullVariantColumnKept(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Payload"],
		"coltype": {"Payload": "VARIANT"},
		"collabel": {"Payload": "Payload"},
		"rows": [ {"Payload": null} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)
	frame := BuildDataFrame(m, nil)

	// Every row is null, so no underlying type was ever seen — the column must still render
	// (one all-null sub-column), not be dropped from the frame.
	if len(frame.Fields) != 1 {
		t.Fatalf("want 1 (all-null) sub-column, got %d", len(frame.Fields))
	}
}

func TestBuildDataFrame_MalformedVariantRecordsIssue(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Payload"],
		"coltype": {"Payload": "VARIANT"},
		"collabel": {"Payload": "Payload"},
		"rows": [ {"Payload": "not-an-envelope"} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)
	BuildDataFrame(m, nil)

	if len(m.Issues.List()) == 0 {
		t.Error("a VARIANT value that isn't a {data-type, value} object should record a parse issue")
	}
}
