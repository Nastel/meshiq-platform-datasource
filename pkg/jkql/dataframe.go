package jkql

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// BuildDataFrame turns a DataModel into a Grafana data frame.
func BuildDataFrame(model DataModel) *data.Frame {
	frame := data.NewFrame("")

	x := 0
	for _, header := range model.Headers {
		columnDataType := model.DataTypes[header]
		if columnDataType == VARIANT {
			addVariantColumn(frame, model, &x, header)
		} else {
			addSimpleColumn(frame, model, &x, columnDataType, header)
		}
	}

	frame.Meta = buildFrameMeta(model)
	return frame
}

// buildFrameMeta attaches result metadata to a freshly built frame: a notice for any in-body
// WARNING/ERROR status, row statistics for the query inspector, and a default Table
// visualization hint.
func buildFrameMeta(model DataModel) *data.FrameMeta {
	meta := &data.FrameMeta{
		Type:                   data.FrameTypeTable,
		PreferredVisualization: data.VisTypeTable,
		Stats: []data.QueryStat{
			{FieldConfig: data.FieldConfig{DisplayName: "Rows returned"}, Value: float64(model.RowCount)},
			{FieldConfig: data.FieldConfig{DisplayName: "Total rows matched"}, Value: float64(model.TotalRowCount)},
		},
	}
	if notice := buildStatusNotice(model); notice != nil {
		meta.Notices = append(meta.Notices, *notice)
	}
	// Parse issues are tolerated (values nulled, columns skipped) but never hidden: the user
	// sees this warning on the panel, and the caller logs the details.
	if issues := model.Issues.List(); len(issues) > 0 {
		meta.Notices = append(meta.Notices, data.Notice{
			Severity: data.NoticeSeverityWarning,
			Text: fmt.Sprintf("%d value(s) could not be read from the dataservice response and are shown as empty. "+
				"Details are in the Grafana server log.", len(issues)),
		})
	}
	return meta
}

// buildStatusNotice turns the result set's in-body status/status-msg into a panel notice, so
// warnings like "Only returning N of M rows" surface in the UI instead of being silently dropped.
func buildStatusNotice(model DataModel) *data.Notice {
	if model.StatusMsg == "" && model.Status != WARNING && model.Status != ERROR {
		return nil
	}

	severity := data.NoticeSeverityInfo
	switch model.Status {
	case WARNING:
		severity = data.NoticeSeverityWarning
	case ERROR:
		severity = data.NoticeSeverityError
	}

	text := model.StatusMsg
	if text == "" {
		text = model.Status
	}
	return &data.Notice{Severity: severity, Text: text}
}

// Frame formats FinalizeFrame accepts (the query editor's "Format as" option).
const (
	FormatTable      = "table"
	FormatTimeSeries = "timeseries"
)

// FinalizeFrame applies query-level metadata after a frame is built: the executed jKQL (shown in
// the query inspector) and the requested format — "timeseries" reshapes to a time-series frame,
// anything else stays a table. It takes primitives rather than the query model so the converter
// stays independent of the query layer.
func FinalizeFrame(frame *data.Frame, executedQuery string, format string) *data.Frame {
	if frame.Meta == nil {
		frame.Meta = &data.FrameMeta{}
	}
	frame.Meta.ExecutedQueryString = executedQuery

	if format == FormatTimeSeries {
		frame = toTimeSeries(frame)
	}
	return frame
}

// toTimeSeries reshapes a frame for graphing when "Format as: Time series" is selected. Long
// results (time + value + label columns) are pivoted to wide multi-series via LongToWide; already
// wide results pass through; anything that isn't a time series stays a table with an explanation.
func toTimeSeries(frame *data.Frame) *data.Frame {
	schema := frame.TimeSeriesSchema()
	switch schema.Type {
	case data.TimeSeriesTypeLong:
		sortFrameByTime(frame, schema.TimeIndex)
		wide, err := data.LongToWide(frame, nil)
		if err != nil {
			addNotice(frame, data.NoticeSeverityWarning, "Could not format as time series: "+err.Error()+".")
			return frame
		}
		if wide.Meta == nil {
			wide.Meta = &data.FrameMeta{}
		}
		wide.Meta.Type = data.FrameTypeTimeSeriesWide
		wide.Meta.PreferredVisualization = data.VisTypeGraph
		return wide
	case data.TimeSeriesTypeWide:
		sortFrameByTime(frame, schema.TimeIndex)
		frame.Meta.Type = data.FrameTypeTimeSeriesWide
		frame.Meta.PreferredVisualization = data.VisTypeGraph
		return frame
	default:
		addNotice(frame, data.NoticeSeverityInfo,
			"Time series format needs a time field and at least one numeric field; showing as table.")
		return frame
	}
}

// sortFrameByTime stably reorders every row of a frame by the time field (ascending). jKQL results
// aren't guaranteed to be time-ordered, and Grafana time series need monotonic time (LongToWide
// requires it, and graph panels render unsorted points as zig-zags).
func sortFrameByTime(frame *data.Frame, timeIndex int) {
	if timeIndex < 0 || timeIndex >= len(frame.Fields) {
		return
	}

	timeField := frame.Fields[timeIndex]
	n := timeField.Len()
	if n < 2 {
		return
	}

	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return timeBefore(timeField.At(order[a]), timeField.At(order[b]))
	})

	// Skip the row copy if already sorted.
	alreadySorted := true
	for i, idx := range order {
		if i != idx {
			alreadySorted = false
			break
		}
	}
	if alreadySorted {
		return
	}

	for _, field := range frame.Fields {
		reordered := make([]interface{}, n)
		for newIdx, oldIdx := range order {
			reordered[newIdx] = field.CopyAt(oldIdx)
		}
		for i := 0; i < n; i++ {
			field.Set(i, reordered[i])
		}
	}
}

// timeBefore orders time values ascending; null/unknown times sort first.
func timeBefore(a, b interface{}) bool {
	ta, aok := asTime(a)
	tb, bok := asTime(b)
	if !aok {
		return bok
	}
	if !bok {
		return false
	}
	return ta.Before(tb)
}

func asTime(v interface{}) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, true
	default:
		return time.Time{}, false
	}
}

func addNotice(frame *data.Frame, severity data.NoticeSeverity, text string) {
	if frame.Meta == nil {
		frame.Meta = &data.FrameMeta{}
	}
	frame.Meta.Notices = append(frame.Meta.Notices, data.Notice{Severity: severity, Text: text})
}

// addVariantColumn splits a VARIANT column into one sub-column per underlying type actually seen
// in the rows (collapsed to DECIMAL/STRING/BOOLEAN/TIMESTAMP/TIMEINTERVAL — see
// getVariantDataTypeAndValue), so e.g. a column that mixes numbers and text becomes a numeric
// sub-column and a text sub-column rather than one stringified column.
func addVariantColumn(frame *data.Frame, model DataModel, x *int, header string) {
	types := getUniqueVariantTypes(model, header)
	if len(types) == 0 {
		// Every row is null, so no underlying type was seen. Still render the column (one
		// all-null string sub-column) instead of silently dropping it from the frame.
		types = []string{STRING}
	}
	for t := range types {
		frameDataType := types[t]
		addSimpleColumn(frame, model, x, frameDataType, header)
	}
}

func getUniqueVariantTypes(model DataModel, header string) []string {
	types := make([]string, 0)
	for _, row := range model.Rows {
		variant := row[header]
		if variant == nil {
			continue
		}
		envelope, ok := variant.(map[string]interface{})
		if !ok {
			model.Issues.Add(fmt.Sprintf("column %q: variant value is not a {data-type, value} object", header))
			continue
		}
		dataType, _ := getVariantDataTypeAndValue(envelope)
		if !Contains(types, dataType) {
			types = append(types, dataType)
		}
	}
	return types
}

func getVariantDataTypeAndValue(variant map[string]interface{}) (string, interface{}) {
	value := variant["value"]
	// A missing or mistyped data-type falls through to STRING, the tolerant default.
	dataType, _ := variant["data-type"].(string)
	switch dataType {
	case INTEGER, DECIMAL:
		dataType = DECIMAL
	case BOOLEAN, TIMESTAMP, TIMEINTERVAL:
		// Keep these as their own real type instead of folding into STRING — a boolean/time
		// VARIANT value should render as a real bool/time, not stringified text.
	default:
		dataType = STRING
	}

	return dataType, value
}

func addSimpleColumn(frame *data.Frame, model DataModel, x *int, frameDataType string, header string) {
	addEmptyField(frame, model, frameDataType, header)
	setFrameValues(frame, model, x, frameDataType, header)
}

func addEmptyField(frame *data.Frame, model DataModel, frameDataType string, header string) {
	size := model.RowCount
	label := model.Label[header]
	columnDataType := model.DataTypes[header]
	if columnDataType == VARIANT {
		label += " (" + frameDataType + ")"
	}
	emptyFrameValues := buildFrameDataType(frameDataType, size)
	field := data.NewField(label, nil, emptyFrameValues)
	if frameDataType == TIMEINTERVAL {
		// jKQL time intervals are microseconds; tag the unit so Grafana renders "1.5 ms".
		field.Config = &data.FieldConfig{Unit: "µs"}
	}
	frame.Fields = append(frame.Fields, field)
}

func buildFrameDataType(dataType string, size int) interface{} {
	switch dataType {
	case TIMESTAMP:
		return make([]*time.Time, size)
	case INTEGER, TIMEINTERVAL:
		return make([]*int64, size)
	case BOOLEAN:
		return make([]*bool, size)
	case DECIMAL:
		return make([]*float64, size)
	case STRING, ENUM, LABELSET, CLOB:
		return make([]*string, size)
	default:
		// Any array coltype carries JSON; unknown scalars render as text. Must agree with
		// ConvertToGrafanaValue's default.
		if strings.HasSuffix(dataType, "[]") {
			return make([]*json.RawMessage, size)
		}
		return make([]*string, size)
	}
}

func setFrameValues(frame *data.Frame, model DataModel, x *int, frameDataType string, header string) {
	for y, row := range model.Rows {
		var value interface{}
		columnDataType := model.DataTypes[header]

		if columnDataType == VARIANT {
			envelope, ok := row[header].(map[string]interface{})
			if !ok {
				continue // null, or a bad envelope already recorded by getUniqueVariantTypes
			}
			variantDataType, variantValue := getVariantDataTypeAndValue(envelope)
			// Each value belongs to exactly one sub-column: the one matching its collapsed type
			// (DECIMAL or STRING). Rows of the other type stay null here — a one-sided skip
			// would duplicate numbers into the string sub-column as text.
			if variantDataType != frameDataType {
				continue
			}
			value = variantValue
		} else {
			value = row[header]
		}

		grafanaValue := ConvertToGrafanaValue(value, frameDataType)
		if grafanaValue != nil {
			frame.SetConcrete(*x, y, grafanaValue)
		}
	}
	*x += 1
}

// ConvertToGrafanaValue converts a single result-set value to a concrete Go value that
// matches the frame field type. Numbers arrive as json.Number (result sets are decoded with
// UseNumber so jKQL INTEGER — a Java long — keeps full precision); ToInt64/ToFloat64 also
// accept plain float64 for hand-built values.
func ConvertToGrafanaValue(value interface{}, dataType string) interface{} {
	if value == nil {
		return nil
	}

	switch dataType {
	case TIMESTAMP:
		if micros, ok := ToInt64(value); ok {
			return convertToTimeObject(micros)
		}
		return nil
	case INTEGER, TIMEINTERVAL:
		if n, ok := ToInt64(value); ok {
			return n
		}
		return nil
	case DECIMAL:
		if f, ok := ToFloat64(value); ok {
			return f
		}
		return nil
	case STRING, LABELSET, CLOB:
		return fmt.Sprint(value)
	case ENUM:
		// The wire encodes an enum as "ordinal#name"; keep just the name.
		parts := strings.SplitN(fmt.Sprint(value), "#", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return parts[0]
	case VARIANT:
		// A VARIANT value is a {data-type, value} envelope. This case is hit for VARIANT[]
		// elements (convertToJSONArray descends with elementType "VARIANT"); scalar VARIANT
		// columns are unwrapped earlier in setFrameValues. Collapse to the value's real type
		// via getVariantDataTypeAndValue, matching how scalar VARIANT columns render, instead
		// of stringifying the raw map.
		m, ok := value.(map[string]interface{})
		if !ok {
			return nil
		}
		innerType, innerValue := getVariantDataTypeAndValue(m)
		return ConvertToGrafanaValue(innerValue, innerType)
	case BOOLEAN:
		if b, ok := value.(bool); ok {
			return b
		}
		return nil
	default:
		// Covers array coltypes and anything not enumerated above: render as JSON/text
		// rather than dropping the column or printing Go map/slice syntax.
		if strings.HasSuffix(dataType, "[]") {
			return convertToJSONArray(value, dataType)
		}
		switch value.(type) {
		case map[string]interface{}, []interface{}:
			if raw, err := json.Marshal(value); err == nil {
				return string(raw)
			}
		}
		return fmt.Sprint(value)
	}
}

// convertToJSONArray marshals an array-typed column value into a json.RawMessage so it can be
// carried in a FieldTypeNullableJSON frame field. Each element is normalized via the element's
// scalar type (timestamps -> RFC3339, numbers -> JSON numbers, etc.).
func convertToJSONArray(value interface{}, arrType string) interface{} {
	slice, ok := value.([]interface{})
	if !ok {
		return nil
	}

	elementType := strings.TrimSuffix(arrType, "[]")
	converted := make([]interface{}, 0, len(slice))
	for _, element := range slice {
		converted = append(converted, ConvertToGrafanaValue(element, elementType))
	}

	raw, err := json.Marshal(converted)
	if err != nil {
		return nil
	}
	return json.RawMessage(raw)
}

// convertToTimeObject converts a jKQL timestamp (microseconds since the epoch) to a time.Time,
// keeping the full microsecond precision — events within the same second must stay ordered.
// The value is normalized to UTC so JSON renderings don't depend on the plugin host's timezone;
// the instant itself is unchanged and Grafana displays it in the dashboard's timezone regardless.
func convertToTimeObject(value int64) time.Time {
	return time.UnixMicro(value).UTC()
}
