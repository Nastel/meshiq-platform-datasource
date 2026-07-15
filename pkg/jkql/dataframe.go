package jkql

import (
	"encoding/json"
	"fmt"
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
		addSimpleColumn(frame, model, &x, columnDataType, header)
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

// FinalizeFrame applies query-level metadata after a frame is built: the executed jKQL, shown in
// the query inspector.
func FinalizeFrame(frame *data.Frame, executedQuery string) *data.Frame {
	if frame.Meta == nil {
		frame.Meta = &data.FrameMeta{}
	}
	frame.Meta.ExecutedQueryString = executedQuery
	return frame
}

func addSimpleColumn(frame *data.Frame, model DataModel, x *int, frameDataType string, header string) {
	addEmptyField(frame, model, frameDataType, header)
	setFrameValues(frame, model, x, frameDataType, header)
}

func addEmptyField(frame *data.Frame, model DataModel, frameDataType string, header string) {
	size := model.RowCount
	label := model.Label[header]
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
	case STRING, ENUM, CLOB:
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
		value := row[header]
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
	case STRING, CLOB:
		return fmt.Sprint(value)
	case ENUM:
		// The wire encodes an enum as "ordinal#name"; keep just the name.
		parts := strings.SplitN(fmt.Sprint(value), "#", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return parts[0]
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
