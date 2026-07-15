package jkql

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// TestFinalizeFrame_TimeSeries_PivotsLongToWide verifies "Format as: Time series" reshapes a
// long-format result (time + numeric value + a label column) into a wide multi-series frame via
// LongToWide.
func TestFinalizeFrame_TimeSeries_PivotsLongToWide(t *testing.T) {
	t0 := time.Unix(1000, 0).UTC()
	t1 := time.Unix(2000, 0).UTC()

	frame := data.NewFrame("",
		data.NewField("Time", nil, []time.Time{t0, t0, t1, t1}),
		data.NewField("Value", nil, []float64{1, 2, 3, 4}),
		data.NewField("Host", nil, []string{"a", "b", "a", "b"}),
	)

	result := FinalizeFrame(frame, "Get Events", FormatTimeSeries)
	if result.Meta.Type != data.FrameTypeTimeSeriesWide {
		t.Fatalf("frame type = %v, want %v", result.Meta.Type, data.FrameTypeTimeSeriesWide)
	}
	// One time field plus one value field per distinct Host ("a", "b").
	if len(result.Fields) != 3 {
		t.Fatalf("expected 3 fields (time + 2 series) after LongToWide, got %d", len(result.Fields))
	}
}

// TestFinalizeFrame_TimeSeries_RestoresEnumConfig is the LongToWide enum-config workaround: Grafana's
// LongToWide keeps an enum field's values but drops its EnumFieldConfig.Text table, which crashes
// graphing (it's read with a non-null assertion). This pins that the dense Text table survives the
// pivot.
func TestFinalizeFrame_TimeSeries_RestoresEnumConfig(t *testing.T) {
	t0 := time.Unix(1000, 0).UTC()
	t1 := time.Unix(2000, 0).UTC()

	one := data.EnumItemIndex(1)
	enumField := data.NewField("Severity", nil, []*data.EnumItemIndex{&one, &one})
	enumField.SetConfig(&data.FieldConfig{
		TypeConfig: &data.FieldTypeConfig{Enum: &data.EnumFieldConfig{Text: []string{"INFO", "ERROR"}}},
	})

	frame := data.NewFrame("",
		data.NewField("Time", nil, []time.Time{t0, t1}),
		data.NewField("Value", nil, []float64{1, 2}),
		enumField,
	)

	result := FinalizeFrame(frame, "Get Events", FormatTimeSeries)

	var found *data.Field
	for _, f := range result.Fields {
		if f.Name == "Severity" {
			found = f
			break
		}
	}
	if found == nil {
		t.Fatal("expected a Severity field to survive the pivot")
	}
	if found.Config == nil || found.Config.TypeConfig == nil || found.Config.TypeConfig.Enum == nil {
		t.Fatal("expected the Severity field to keep its enum TypeConfig after LongToWide")
	}
	if len(found.Config.TypeConfig.Enum.Text) != 2 {
		t.Errorf("Severity enum Text table = %v, want the original 2-entry table restored", found.Config.TypeConfig.Enum.Text)
	}
}

// TestFinalizeFrame_Table_DoesNotPivot verifies the default ("table") format leaves the frame
// untouched — no LongToWide, no time-series metadata.
func TestFinalizeFrame_Table_DoesNotPivot(t *testing.T) {
	frame := data.NewFrame("",
		data.NewField("Name", nil, []string{"a", "b"}),
	)
	result := FinalizeFrame(frame, "Get Events", FormatTable)
	if result.Meta.Type == data.FrameTypeTimeSeriesWide {
		t.Error("table format must not be reshaped into a time series")
	}
	if len(result.Fields) != 1 {
		t.Errorf("expected the single field unchanged, got %d fields", len(result.Fields))
	}
}
