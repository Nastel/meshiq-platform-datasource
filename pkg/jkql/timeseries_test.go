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
