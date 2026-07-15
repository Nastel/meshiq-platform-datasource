package jkql

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// EnumResolver returns the complete, ordinal-indexed value set of a built-in enum field (index =
// jKQL ordinal, value = name), or nil/empty when it can't be resolved. Used to build a full enum
// Text table so legends/value-mappings show every possible value, not just those in the result.
type EnumResolver func(field string) []string

// BuildDataFrame turns a DataModel into a Grafana data frame. enumValues may be nil; when
// provided it supplies the complete value set for enum columns (see addEnumColumn).
func BuildDataFrame(model DataModel, enumValues EnumResolver) *data.Frame {
	frame := data.NewFrame("")

	x := 0
	for _, header := range model.Headers {
		columnDataType := model.DataTypes[header]
		switch columnDataType {
		case VARIANT:
			addVariantColumn(frame, model, &x, header)
		case ENUM:
			addEnumColumn(frame, model, &x, header, enumValues)
		default:
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
		restoreEnumConfigs(wide, frame)
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

// restoreEnumConfigs re-attaches enum type configs to a wide frame after LongToWide, which preserves
// each enum field's values and type but DROPS its EnumFieldConfig.Text table (leaving config: {}).
// Without the table, graphing the wide frame crashes Grafana's enum handling, which reads
// field.config.type.enum with a non-null assertion. Configs are matched to the original long frame's
// fields by name (enum fields with the same name share the same enum type, so the same table).
//
// Self-deactivating: it only touches enum fields whose TypeConfig is MISSING. If a future SDK's
// LongToWide starts preserving TypeConfig (the upstream fix), every wide enum field will already
// carry it, hasMissingEnumConfig returns false, and this becomes a no-op — no overwriting, no need
// to remove the workaround.
func restoreEnumConfigs(wide, long *data.Frame) {
	if !hasMissingEnumConfig(wide) {
		return
	}

	configs := make(map[string]*data.FieldTypeConfig)
	for _, f := range long.Fields {
		if isEnumField(f) && f.Config != nil && f.Config.TypeConfig != nil {
			configs[f.Name] = f.Config.TypeConfig
		}
	}

	for _, f := range wide.Fields {
		// Skip fields that already carry their type config (the state a fixed LongToWide would leave).
		if !isEnumField(f) || (f.Config != nil && f.Config.TypeConfig != nil) {
			continue
		}
		typeConfig, ok := configs[f.Name]
		if !ok {
			continue
		}
		if f.Config == nil {
			f.Config = &data.FieldConfig{}
		}
		f.Config.TypeConfig = typeConfig
	}
}

// hasMissingEnumConfig reports whether any enum field in the frame is missing its TypeConfig — the
// condition restoreEnumConfigs exists to repair. Returns false once LongToWide preserves it.
func hasMissingEnumConfig(frame *data.Frame) bool {
	for _, f := range frame.Fields {
		if isEnumField(f) && (f.Config == nil || f.Config.TypeConfig == nil) {
			return true
		}
	}
	return false
}

func isEnumField(f *data.Field) bool {
	return f.Type() == data.FieldTypeEnum || f.Type() == data.FieldTypeNullableEnum
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

// addEnumColumn builds a native Grafana Enum field (FieldTypeNullableEnum). Grafana's enum and
// time-series code treats the EnumFieldConfig.Text table as a DENSE value set (one legend/axis entry
// per slot) and assumes no gaps, so we cannot key Text by the raw jKQL ordinal (which is sparse: a
// column showing only "INFO" at ordinal 3 would emit ["","","","INFO"], and the empty placeholder
// slots break graphing).
//
// When enumValues resolves the field's complete value set (via "GET ENUMERATION FOR <field>"), we
// use it directly as the Text table (indexed by ordinal — jKQL enum ordinals are contiguous, so it
// is dense) and store each row's raw ordinal. This exposes every possible value in legends/filters,
// not just those present in the result. Otherwise we fall back to remapping the distinct ordinals
// actually seen to a compact 0..N-1 index, which is still gap-free.
func addEnumColumn(frame *data.Frame, model DataModel, x *int, header string, enumValues EnumResolver) {
	if text := resolveFullEnumText(model, header, enumValues); len(text) > 0 {
		values := make([]*data.EnumItemIndex, model.RowCount)
		for y, row := range model.Rows {
			enum, ok := row[header].(JkqlEnum)
			if !ok {
				continue // null enum cells are normal
			}
			if enum.Ordinal < 0 || enum.Ordinal >= len(text) {
				model.Issues.Add(fmt.Sprintf("column %s: enum ordinal %d is outside the resolved value table (%d values)", header, enum.Ordinal, len(text)))
				continue
			}
			v := data.EnumItemIndex(enum.Ordinal)
			values[y] = &v
		}
		appendEnumField(frame, columnFieldName(model, header), model.Label[header], values, text)
		*x++
		return
	}

	// Fallback: compact the distinct enum values seen in this column to a gap-free 0..N-1 table.
	// Keyed by the whole (ordinal, name) pair, not the ordinal alone: malformed wire values all
	// parse to ordinal 0, and ordinal-only keying would display the first one's text for all.
	compact := make(map[JkqlEnum]data.EnumItemIndex)
	text := make([]string, 0)
	values := make([]*data.EnumItemIndex, model.RowCount)
	for y, row := range model.Rows {
		enum, ok := row[header].(JkqlEnum)
		if !ok {
			continue
		}
		index, seen := compact[enum]
		if !seen {
			index = data.EnumItemIndex(len(text))
			compact[enum] = index
			text = append(text, enum.Name)
		}
		v := index
		values[y] = &v
	}
	appendEnumField(frame, columnFieldName(model, header), model.Label[header], values, text)
	*x++
}

// columnFieldName returns the column's underlying jKQL field name (model.Names), falling back to
// the raw header when there is no mapped name. Used to key color rules and enum lookups.
func columnFieldName(model DataModel, header string) string {
	if name := model.Names[header]; name != "" {
		return name
	}
	return header
}

// resolveFullEnumText asks the resolver for the field's complete value set. The queried field is the
// column's underlying jKQL field name (not its alias); a non-identifier name (e.g. a function or
// custom expression) is skipped since it isn't a built-in enum type.
func resolveFullEnumText(model DataModel, header string, enumValues EnumResolver) []string {
	if enumValues == nil {
		return nil
	}
	field := columnFieldName(model, header)
	if !IdentifierRegExp.MatchString(field) {
		return nil
	}
	return enumValues(field)
}

func appendEnumField(frame *data.Frame, fieldName, label string, values []*data.EnumItemIndex, text []string) {
	field := data.NewField(label, nil, values)
	colors := enumColors(fieldName, text)
	enumConfig := &data.EnumFieldConfig{Text: text}
	if colors != nil {
		enumConfig.Color = colors
	}
	config := &data.FieldConfig{
		TypeConfig: &data.FieldTypeConfig{Enum: enumConfig},
	}
	// Grafana's display processor uses config.type.enum.color values as-is, without resolving
	// named theme colors (e.g. "semi-dark-red") the way it does for value mappings — so table/stat
	// cells relying on that array render unstyled. Mirror the same colors as mappings (keyed by
	// ordinal, checked before the enum branch, and theme-resolved) so the coloring actually shows.
	if colors != nil {
		mapper := data.ValueMapper{}
		for i, name := range text {
			mapper[strconv.Itoa(i)] = data.ValueMappingResult{Text: name, Color: colors[i]}
		}
		config.Mappings = data.ValueMappings{mapper}
	}
	field.SetConfig(config)
	frame.Fields = append(frame.Fields, field)
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
	// Color recognized string/labelset values (e.g. Allow -> green, Deny -> red) via value
	// mappings. Only real STRING/LABELSET columns qualify, not VARIANT sub-columns.
	if columnDataType == STRING || columnDataType == LABELSET {
		if mappings := stringValueMappings(model, header); mappings != nil {
			if field.Config == nil {
				field.Config = &data.FieldConfig{}
			}
			field.Config.Mappings = mappings
		}
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
		// Any array or map coltype (enumerated or not, e.g. MAP(STRING[])) carries JSON;
		// unknown scalars render as text. Must agree with ConvertToGrafanaValue's default.
		if strings.HasSuffix(dataType, "[]") || dataType == MAP || strings.HasPrefix(dataType, "MAP(") {
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
			// Each value belongs to exactly one sub-column: the one matching its variant type.
			// Rows of other types stay null here — a one-sided skip would duplicate numbers
			// into the string sub-column as text.
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
		// Columns wrap enums in the data model; raw wire values ("ordinal#name") reach this
		// case as elements of maps/arrays the model keeps unexploded.
		if e, ok := value.(JkqlEnum); ok {
			return e.Name
		}
		return ToEnumObject(value).Name
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
		// Covers every array and map coltype (enumerated or not, e.g. MAP(STRING[])) plus
		// unknown types. Containers of unknown type render as JSON text, never Go syntax.
		switch {
		case strings.HasSuffix(dataType, "[]"):
			return convertToJSONArray(value, dataType)
		case dataType == MAP || strings.HasPrefix(dataType, "MAP("):
			return convertToJSONMap(value, dataType)
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

// convertToJSONMap marshals a non-exploded MAP column value into a json.RawMessage. For typed maps
// each value is normalized by its element type (timestamps -> RFC3339, etc.); untyped MAP values
// are already scalar and marshaled as-is. json.Marshal sorts keys, so output is stable.
func convertToJSONMap(value interface{}, dataType string) interface{} {
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}

	if strings.HasPrefix(dataType, "MAP(") && strings.HasSuffix(dataType, ")") {
		// Derive the element type from the coltype itself so nested shapes work too:
		// MAP(STRING[]) -> STRING[] elements render as JSON arrays, not stringified slices.
		elementType := strings.TrimSuffix(strings.TrimPrefix(dataType, "MAP("), ")")
		normalized := make(map[string]interface{}, len(m))
		for k, v := range m {
			normalized[k] = ConvertToGrafanaValue(v, elementType)
		}
		m = normalized
	}

	raw, err := json.Marshal(m)
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
