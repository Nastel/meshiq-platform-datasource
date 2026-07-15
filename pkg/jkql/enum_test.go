package jkql

import (
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// enumResultSet builds a minimal parsed result set with one ENUM column ("Severity"), one row per
// given wire value ("ordinal#name").
func enumResultSet(wireValues ...string) map[string]interface{} {
	rows := make([]interface{}, len(wireValues))
	for i, v := range wireValues {
		rows[i] = map[string]interface{}{"Severity": v}
	}
	return map[string]interface{}{
		"row-count": len(wireValues), "total-row-count": len(wireValues), "status": "SUCCESS",
		"colhdr":   []interface{}{"Severity"},
		"coltype":  map[string]interface{}{"Severity": ENUM},
		"collabel": map[string]interface{}{"Severity": "Severity"},
		"rows":     rows,
	}
}

func TestBuildDataModel_EnumColumn_WrapsValues(t *testing.T) {
	model := BuildDataModel(enumResultSet("3#INFO", "5#ERROR"), nil)

	if model.DataTypes["Severity"] != ENUM {
		t.Fatalf("data type = %q, want ENUM", model.DataTypes["Severity"])
	}
	if model.Names["Severity"] != "Severity" {
		t.Errorf("Names[Severity] = %q, want %q", model.Names["Severity"], "Severity")
	}
	enum0, ok := model.Rows[0]["Severity"].(JkqlEnum)
	if !ok || enum0.Ordinal != 3 || enum0.Name != "INFO" {
		t.Errorf("row 0 = %#v, want JkqlEnum{3, INFO}", model.Rows[0]["Severity"])
	}
	enum1, ok := model.Rows[1]["Severity"].(JkqlEnum)
	if !ok || enum1.Ordinal != 5 || enum1.Name != "ERROR" {
		t.Errorf("row 1 = %#v, want JkqlEnum{5, ERROR}", model.Rows[1]["Severity"])
	}
}

func TestBuildDataModel_EnumColumn_NullStaysNull(t *testing.T) {
	parsed := enumResultSet()
	parsed["row-count"] = 1
	parsed["total-row-count"] = 1
	parsed["rows"] = []interface{}{map[string]interface{}{"Severity": nil}}

	model := BuildDataModel(parsed, nil)
	if model.Rows[0]["Severity"] != nil {
		t.Errorf("null enum value should stay nil, got %#v", model.Rows[0]["Severity"])
	}
}

// TestBuildDataFrame_EnumColumn_DenseFromResolver verifies that when an EnumResolver supplies the
// field's complete value set, the frame gets a native enum field keyed by the raw ordinal against
// that dense table — not a remapped compact index.
func TestBuildDataFrame_EnumColumn_DenseFromResolver(t *testing.T) {
	model := BuildDataModel(enumResultSet("3#INFO", "5#ERROR"), nil)

	fullText := []string{"NONE", "TRACE", "DEBUG", "INFO", "NOTICE", "ERROR"} // dense, ordinals 0-5
	resolver := func(field string) []string {
		if field == "Severity" {
			return fullText
		}
		return nil
	}

	frame := BuildDataFrame(model, resolver)
	if len(frame.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(frame.Fields))
	}
	field := frame.Fields[0]
	if field.Type() != data.FieldTypeNullableEnum {
		t.Fatalf("field type = %v, want FieldTypeNullableEnum", field.Type())
	}
	enumConfig := field.Config.TypeConfig.Enum
	if len(enumConfig.Text) != len(fullText) {
		t.Fatalf("Text table len = %d, want %d (the full dense set, not just seen values)", len(enumConfig.Text), len(fullText))
	}
	// Row 0 held ordinal 3 -> stored as index 3 into the dense table.
	v0 := field.At(0).(*data.EnumItemIndex)
	if int(*v0) != 3 {
		t.Errorf("row 0 index = %d, want 3 (the raw ordinal)", *v0)
	}
}

// TestBuildDataFrame_EnumColumn_FallsBackToCompactTable verifies that without a resolver (or when
// it returns nothing), the frame still builds a gap-free enum by compacting the distinct ordinals
// actually seen — this must not panic or emit sparse placeholder text.
func TestBuildDataFrame_EnumColumn_FallsBackToCompactTable(t *testing.T) {
	model := BuildDataModel(enumResultSet("7#WARNING", "20#ERROR"), nil)

	frame := BuildDataFrame(model, nil)
	field := frame.Fields[0]
	enumConfig := field.Config.TypeConfig.Enum
	if len(enumConfig.Text) != 2 {
		t.Fatalf("compact Text table len = %d, want 2 (only the distinct ordinals seen)", len(enumConfig.Text))
	}
	// Whichever compact indices were assigned, the two names must both be present with no blanks.
	seen := map[string]bool{}
	for _, name := range enumConfig.Text {
		if name == "" {
			t.Fatalf("compact Text table must not contain empty placeholder slots: %v", enumConfig.Text)
		}
		seen[name] = true
	}
	if !seen["WARNING"] || !seen["ERROR"] {
		t.Errorf("expected WARNING and ERROR in the compact table, got %v", enumConfig.Text)
	}
}

// TestBuildDataFrame_EnumColumn_ColorsFromRule verifies a field with a coloring rule (Severity)
// gets its enum Color table populated, mirrored into value mappings so table/stat cells render it.
func TestBuildDataFrame_EnumColumn_ColorsFromRule(t *testing.T) {
	model := BuildDataModel(enumResultSet("0#INFO", "1#ERROR"), nil)
	resolver := func(string) []string { return []string{"INFO", "ERROR"} }

	frame := BuildDataFrame(model, resolver)
	field := frame.Fields[0]
	enumConfig := field.Config.TypeConfig.Enum
	if enumConfig.Color == nil {
		t.Fatal("expected a color table for the Severity enum")
	}
	if enumConfig.Color[0] != colorBlue || enumConfig.Color[1] != colorRed {
		t.Errorf("colors = %v, want [%q, %q]", enumConfig.Color, colorBlue, colorRed)
	}
	if field.Config.Mappings == nil {
		t.Fatal("expected value mappings mirroring the enum colors (table/stat cells read these, not config.type.enum.color)")
	}
}
