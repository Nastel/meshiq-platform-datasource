package jkql

import (
	"encoding/json"
	"math"
	"testing"
)

func TestToInt64(t *testing.T) {
	cases := []struct {
		name   string
		value  interface{}
		want   int64
		wantOk bool
	}{
		{"json.Number", json.Number("42"), 42, true},
		{"big json.Number keeps precision", json.Number("9007199254740993"), 9007199254740993, true},
		{"float64", float64(7), 7, true},
		{"int64", int64(9), 9, true},
		{"int", 3, 3, true},
		{"unsupported type", "not a number", 0, false},
		// MaxInt64 exactly still parses via json.Number's own Int64 (it fits a Java long, the
		// jKQL INTEGER type), so the range guard must not reject a legitimate boundary value.
		{"json.Number MaxInt64 still ok", json.Number("9223372036854775807"), 9223372036854775807, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ToInt64(c.value)
			if got != c.want || ok != c.wantOk {
				t.Errorf("ToInt64(%v) = (%d, %v), want (%d, %v)", c.value, got, ok, c.want, c.wantOk)
			}
		})
	}
}

// TestToInt64_OutOfRangeFloatRejected pins that a float value outside the int64 range is rejected
// (ok=false) instead of returning a garbage int64. A Go conversion of an out-of-range float is
// implementation-defined (e.g. 1e300 lands on MinInt64), which would turn a nonsense wire value
// into a confidently wrong number rather than a null.
func TestToInt64_OutOfRangeFloatRejected(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
	}{
		{"json.Number too large", json.Number("1e300")},
		{"float64 too large", float64(1e300)},
		{"float64 too small", float64(-1e300)},
		{"NaN", math.NaN()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, ok := ToInt64(c.value); ok {
				t.Errorf("ToInt64(%v) = (%d, true), want ok=false for an out-of-range value", c.value, got)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	cases := []struct {
		name   string
		value  interface{}
		want   float64
		wantOk bool
	}{
		{"json.Number", json.Number("1.5"), 1.5, true},
		{"float64", float64(2.25), 2.25, true},
		{"int64", int64(4), 4, true},
		{"int", 5, 5, true},
		{"unsupported type", "not a number", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ToFloat64(c.value)
			if got != c.want || ok != c.wantOk {
				t.Errorf("ToFloat64(%v) = (%v, %v), want (%v, %v)", c.value, got, ok, c.want, c.wantOk)
			}
		})
	}
}
