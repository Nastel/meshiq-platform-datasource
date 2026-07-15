package jkql

import (
	"encoding/json"
	"math"
)

// ToInt64 normalizes a wire numeric value to an int64. Result sets are decoded with
// json.Decoder.UseNumber, so numbers arrive as json.Number — which keeps the full range of a
// Java long (the jKQL INTEGER type) exactly, where float64 loses precision past 2^53. float64
// and int are accepted too (hand-built values in tests, callers that decoded plainly).
func ToInt64(value interface{}) (int64, bool) {
	switch n := value.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		if f, err := n.Float64(); err == nil {
			return float64ToInt64(f)
		}
		return 0, false
	case float64:
		return float64ToInt64(n)
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

// float64ToInt64 converts f only when it fits in an int64 — Go leaves the out-of-range conversion
// implementation-defined, which would turn a garbage wire value into a confidently wrong number
// instead of a null. float64(MaxInt64) rounds up to 2^63, exactly the first out-of-range value,
// so >= is the right upper test; NaN would slip past both comparisons.
func float64ToInt64(f float64) (int64, bool) {
	if math.IsNaN(f) || f < math.MinInt64 || f >= math.MaxInt64 {
		return 0, false
	}
	return int64(f), true
}

// ToFloat64 normalizes a wire numeric value to a float64 (the jKQL DECIMAL type is a Java
// double, so float64 is exact). Accepts the same input kinds as ToInt64.
func ToFloat64(value interface{}) (float64, bool) {
	switch n := value.(type) {
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f, true
		}
		return 0, false
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
