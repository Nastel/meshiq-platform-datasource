package jkql

import (
	"reflect"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func TestEnumColors(t *testing.T) {
	cases := []struct {
		name  string
		field string
		text  []string
		want  []string
	}{
		{"severity meanings", "Severity", []string{"NONE", "INFO", "WARNING", "ERROR"},
			[]string{colorGreen, colorBlue, colorOrange, colorRed}},
		{"compcode", "CompCode", []string{"SUCCESS", "WARNING", "ERROR"},
			[]string{colorGreen, colorOrange, colorRed}},
		{"field name is case-insensitive, values too", "severity", []string{"info", "error"},
			[]string{colorBlue, colorRed}},
		{"partial match keeps blanks for unknowns", "Severity", []string{"INFO", "MYSTERY"},
			[]string{colorBlue, ""}},
		// UNKNOWN is colored gray, not left blank or given a palette color meant for a real
		// error state — an unresolved value isn't a warning.
		{"UNKNOWN is gray, not orange or blank", "Severity", []string{"INFO", "UNKNOWN"},
			[]string{colorBlue, colorGray}},
		{"unknown field is not colored", "Host", []string{"a", "b"}, nil},
		{"known field but no known values", "Severity", []string{"MYSTERY"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := enumColors(c.field, c.text)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("enumColors(%q, %v) = %v, want %v", c.field, c.text, got, c.want)
			}
		})
	}
}

func TestStringValueMappings(t *testing.T) {
	// A string column whose values are in the default two-state palette gets colored.
	model := DataModel{
		ItemType:  "Log",
		Headers:   []string{"Action"},
		Names:     map[string]string{"Action": "Action"},
		DataTypes: map[string]DataType{"Action": STRING},
		Rows: []map[string]interface{}{
			{"Action": "Allowed"}, {"Action": "Denied"}, {"Action": "Allowed"},
		},
	}
	vms := stringValueMappings(model, "Action")
	if vms == nil {
		t.Fatal("expected value mappings for Allowed/Denied")
	}
	mapper, ok := vms[0].(data.ValueMapper)
	if !ok {
		t.Fatalf("expected a ValueMapper, got %T", vms[0])
	}
	if len(mapper) != 2 {
		t.Errorf("mapper size = %d, want 2 (deduped)", len(mapper))
	}
	if mapper["Allowed"].Color != colorGreen {
		t.Errorf("Allowed color = %q, want %q", mapper["Allowed"].Color, colorGreen)
	}
	if mapper["Denied"].Color != colorRed {
		t.Errorf("Denied color = %q, want %q", mapper["Denied"].Color, colorRed)
	}
}

func TestStringValueMappings_UnknownIsGray(t *testing.T) {
	model := DataModel{
		ItemType:  "Log",
		Headers:   []string{"Action"},
		Names:     map[string]string{"Action": "Action"},
		DataTypes: map[string]DataType{"Action": STRING},
		Rows:      []map[string]interface{}{{"Action": "Unknown"}},
	}
	vms := stringValueMappings(model, "Action")
	if vms == nil {
		t.Fatal("expected value mappings for Unknown")
	}
	mapper := vms[0].(data.ValueMapper)
	if mapper["Unknown"].Color != colorGray {
		t.Errorf("Unknown color = %q, want %q (gray, not orange)", mapper["Unknown"].Color, colorGray)
	}
}

func TestStringValueMappings_NoKnownValues(t *testing.T) {
	model := DataModel{
		ItemType:  "Log",
		Headers:   []string{"Host"},
		Names:     map[string]string{"Host": "Host"},
		DataTypes: map[string]DataType{"Host": STRING},
		Rows:      []map[string]interface{}{{"Host": "h1"}, {"Host": "h2"}},
	}
	if vms := stringValueMappings(model, "Host"); vms != nil {
		t.Errorf("expected nil mappings for unrecognized values, got %v", vms)
	}
}
