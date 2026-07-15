package jkql

import (
	"encoding/json"
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
