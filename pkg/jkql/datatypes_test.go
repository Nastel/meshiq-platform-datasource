package jkql

import "testing"

func TestConvertRangeToDt(t *testing.T) {
	cases := map[string]string{
		RANGE_INTEGER:      INTEGER,
		RANGE_DECIMAL:      DECIMAL,
		RANGE_TIMESTAMP:    TIMESTAMP,
		RANGE_TIMEINTERVAL: TIMEINTERVAL,
		RANGE_GENERIC:      UNDEFINED,
		"NOPE":             UNDEFINED,
	}
	for dt, want := range cases {
		if got := ConvertRangeToDt(dt); got != want {
			t.Errorf("ConvertRangeToDt(%q) = %q, want %q", dt, got, want)
		}
	}
}

func TestConvertDtToPrefix(t *testing.T) {
	cases := map[string]string{
		BOOLEAN:      "B",
		INTEGER:      "I",
		DECIMAL:      "D",
		TIMESTAMP:    "T",
		TIMEINTERVAL: "V",
		STRING:       "S",
		ENUM:         "E",
		BINARY:       "X",
		VARIANT:      "A",
		CLOB:         "C",
		LABELSET:     "L",
		"NOPE":       "", // unknown -> empty
	}
	for dt, want := range cases {
		if got := ConvertDtToPrefix(dt); got != want {
			t.Errorf("ConvertDtToPrefix(%q) = %q, want %q", dt, got, want)
		}
	}
}
