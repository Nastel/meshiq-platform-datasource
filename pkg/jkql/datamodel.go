package jkql

import (
	"maps"
	"regexp"
	"slices"
	"strings"
)

// MapAccessRegExp parses a keyed map-field access, Field('key'), into (field, key). In jKQL this
// syntax means a map access and nothing else, so we match the shape rather than enumerate field
// names: there are many map-type fields (Properties, Quota, Statistics, and more, varying by item
// type), and a whitelist would always be incomplete. The key is optional — a bare field name (the
// whole map) matches with an empty key.
//
// usage example:
// "Properties"           -> ["Properties", "Properties", ""]
// "Statistics('MyStat')" -> ["Statistics('MyStat')", "Statistics", "MyStat"]
var MapAccessRegExp = regexp.MustCompile(`^([A-Za-z0-9_.]+)(?:\('(.+)'\))?$`)

// parseMapAccess splits a keyed map-field access like Statistics('MyStat') into its field and key
// (key is "" for a bare field name — the whole map). ok is false when s is not a map access. That
// includes a function call such as Round('x'), which shares the Field('key') shape: a name that is
// a known jKQL function is treated as a call, not a field, so we never misread one as the other.
func parseMapAccess(s string, cat *FunctionCatalog) (field, key string, ok bool) {
	m := MapAccessRegExp.FindStringSubmatch(s)
	if m == nil || cat.names[m[1]] {
		return "", "", false
	}
	return m[1], m[2], true
}

// A named request must always yield a column per key, even when every row is null (the key is
// unset on these rows, or was mistyped); a whole-map request keeps dropping empty exploded columns.
func namedMapKeys(hdr string, cat *FunctionCatalog) []string {
	_, rawKey, ok := parseMapAccess(hdr, cat)
	if !ok || rawKey == "" {
		return nil
	}
	var keys []string
	for _, k := range strings.Split(rawKey, ",") {
		if k = strings.Trim(strings.TrimSpace(k), "'"); k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// addColumn registers one output column's metadata: its header, display label and jKQL data
// type. Row values are filled separately by the caller.
func (dataModel *DataModel) addColumn(header, label, typ string) {
	dataModel.Headers = append(dataModel.Headers, header)
	dataModel.Label[header] = label
	dataModel.DataTypes[header] = typ
}

// rowObject casts one wire row to an object, recording an issue against column hdr when it is
// not one.
func (dataModel *DataModel) rowObject(r interface{}, hdr string) (map[string]interface{}, bool) {
	row, ok := r.(map[string]interface{})
	if !ok {
		dataModel.Issues.Add("column " + hdr + ": row is not an object")
	}
	return row, ok
}

// mapValue reads column hdr of one row as a map, recording an issue when the value is present
// but not a map. A nil/absent value returns ok=false with no issue — null map cells are normal.
func (dataModel *DataModel) mapValue(row map[string]interface{}, hdr string) (map[string]interface{}, bool) {
	value := row[hdr]
	if value == nil {
		return nil, false
	}
	valueMap, ok := value.(map[string]interface{})
	if !ok {
		dataModel.Issues.Add("column " + hdr + ": value is not a map")
	}
	return valueMap, ok
}

// addRawColumn adds a field as a single column holding each row's value as-is. Used for plain
// scalar columns, and as the fallback for coltypes the converter doesn't otherwise special-case
// (untyped MAP fields other than Properties, unknown types).
func (dataModel *DataModel) addRawColumn(hdr string, typ string, lbl string, rows []interface{}) {
	dataModel.addColumn(hdr, lbl, typ)

	for i, r := range rows {
		row, ok := dataModel.rowObject(r, hdr)
		if !ok {
			dataModel.Rows[i][hdr] = nil
			continue
		}
		dataModel.Rows[i][hdr] = row[hdr]
	}
}

// addArrayColumn adds an array-typed column, normalizing each row's value to a slice (null
// becomes an empty slice, a stray scalar a one-element slice) so the frame builder can render
// it as a JSON array.
func (dataModel *DataModel) addArrayColumn(hdr string, typ string, lbl string, rows []interface{}) {
	dataModel.addColumn(hdr, lbl, typ)

	for i, r := range rows {
		row, ok := dataModel.rowObject(r, hdr)
		if !ok {
			dataModel.Rows[i][hdr] = make([]interface{}, 0)
			continue
		}
		value := row[hdr]
		switch v := value.(type) {
		case nil:
			dataModel.Rows[i][hdr] = make([]interface{}, 0)
		case []interface{}:
			dataModel.Rows[i][hdr] = v
		default:
			dataModel.Rows[i][hdr] = []interface{}{v}
		}
	}
}

// Every map explode below runs the same two phases. Scan: walk the rows once to learn the map's
// key set and each key's value type(s) — that is the whole shape discovery, and the only place
// shape violations are recorded. Emit: for each key (and type), decide the column's header and
// label — the naming policy, which is what makes each explode function different — then register
// the column and copy the values with a fill helper.

// scanMixedMapKeys is the scan phase for a plain MAP column, where each key's type varies and is
// reported per row by the column's :_ValueTypes sibling. Returns key -> value types (more than
// one when a key changes type across rows). Violating rows/keys are recorded and skipped here;
// the fill phase nulls them silently.
func (dataModel *DataModel) scanMixedMapKeys(hdr string, rows []interface{}) map[string][]string {
	hdrVT := hdr + SUFIX_VALUETYPES
	columnMap := make(map[string][]string)
	for _, r := range rows {
		row, ok := dataModel.rowObject(r, hdr)
		if !ok {
			continue
		}
		valueMap, ok := dataModel.mapValue(row, hdr)
		if !ok {
			continue
		}
		rowVT, ok := row[hdrVT].(map[string]interface{})
		if !ok {
			dataModel.Issues.Add("column " + hdr + ": missing or invalid " + hdrVT + " sibling")
			continue
		}
		for key := range valueMap {
			dt, ok := rowVT[key].(string)
			if !ok {
				dataModel.Issues.Add("column " + hdr + ": no value type for key " + key)
				continue
			}
			if !Contains(columnMap[key], dt) {
				columnMap[key] = append(columnMap[key], dt)
			}
		}
	}
	return columnMap
}

// scanTypedMapKeys is the scan phase for a typed map column MAP(dt): every key has the map's
// single element type, so no :_ValueTypes sibling is involved.
func (dataModel *DataModel) scanTypedMapKeys(hdr string, dt string, rows []interface{}) map[string][]string {
	columnMap := make(map[string][]string)
	for _, r := range rows {
		row, ok := dataModel.rowObject(r, hdr)
		if !ok {
			continue
		}
		valueMap, ok := dataModel.mapValue(row, hdr)
		if !ok {
			continue
		}
		for key := range valueMap {
			columnMap[key] = []string{dt}
		}
	}
	return columnMap
}

// fillMapColumn is the fill phase for one exploded key of a typed map: copy the key's value from
// each row's map into the output column, leaving null where the row has no usable value. Shape
// violations were already recorded by the scan phase.
func (dataModel *DataModel) fillMapColumn(header, hdr, key string, rows []interface{}) {
	for index, r := range rows {
		dataModel.Rows[index][header] = nil
		row, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		if valueMap, ok := row[hdr].(map[string]interface{}); ok {
			dataModel.Rows[index][header] = valueMap[key]
		}
	}
}

// fillMixedMapColumn is the fill phase for one exploded key+type of a mixed map: a value is
// copied only when the row's :_ValueTypes sibling confirms it has this column's type (the same
// key can hold different types on different rows, one output column each).
func (dataModel *DataModel) fillMixedMapColumn(header, hdr, key, typ string, rows []interface{}) {
	hdrVT := hdr + SUFIX_VALUETYPES
	for index, r := range rows {
		dataModel.Rows[index][header] = nil
		row, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		valueMap, okValue := row[hdr].(map[string]interface{})
		rowVT, okVT := row[hdrVT].(map[string]interface{})
		if !okValue || !okVT {
			continue
		}
		value := valueMap[key]
		if value == nil || rowVT[key] != typ {
			continue
		}
		dataModel.Rows[index][header] = value
	}
}

// explodeMixedMap explodes a plain MAP field (one requested directly, e.g. Properties or
// Properties('a','b')) into one column per key and type. Naming: a single-key access keeps the
// server's label; a whole-map access labels each column by its key and appends the key to the
// header. A " (Type)" suffix disambiguates keys that hold several types.
func (dataModel *DataModel) explodeMixedMap(hdr string, lbl string, rows []interface{}, cat *FunctionCatalog) {
	columnMap := dataModel.scanMixedMapKeys(hdr, rows)

	// A named access must yield a column for every key it lists, even ones with no data in any row
	// (unset, or mistyped) — this also covers the single multi-key field props('A','B','C'). Add any
	// named key the rows did not supply, defaulting its type to STRING; keys with data keep the type
	// resolved from :_ValueTypes above. Whole-map requests name no keys, so they still drop empties.
	for _, key := range namedMapKeys(hdr, cat) {
		if _, ok := columnMap[key]; !ok {
			columnMap[key] = []string{STRING}
		}
	}

	for _, column := range slices.Sorted(maps.Keys(columnMap)) {
		types := columnMap[column]
		addTypeToLabel := len(types) > 1
		for _, typ := range types {
			header := ConvertDtToPrefix(typ) + ":" + hdr
			var label string
			wrappedColumn := "('" + column + "')"
			if strings.HasSuffix(hdr, wrappedColumn) { // single-key access: Field('key')
				label = lbl
			} else { // whole-map access: one column per discovered key
				header += wrappedColumn
				label = column
			}
			if addTypeToLabel {
				label += " (" + CapitalizeStr(typ) + ")"
			}

			dataModel.addColumn(header, label, typ)
			dataModel.fillMixedMapColumn(header, hdr, column, typ, rows)
		}
	}
}

// explodeTypedMap is explodeMixedMap for a typed map field MAP(dt): same naming policy, but every
// column has the map's element type, so no per-type splitting and no " (Type)" label suffix.
func (dataModel *DataModel) explodeTypedMap(hdr string, dt string, lbl string, rows []interface{}, cat *FunctionCatalog) {
	columnMap := dataModel.scanTypedMapKeys(hdr, dt, rows)

	// Add any named key the rows did not supply, so a named access always yields all its columns
	// (see namedMapKeys); keys with data keep the map's element type dt.
	for _, key := range namedMapKeys(hdr, cat) {
		if _, ok := columnMap[key]; !ok {
			columnMap[key] = []string{dt}
		}
	}

	for _, column := range slices.Sorted(maps.Keys(columnMap)) {
		header := ConvertDtToPrefix(dt) + ":" + hdr
		var label string
		wrappedColumn := "('" + column + "')"
		if strings.HasSuffix(hdr, wrappedColumn) { // single-key access: Field('key')
			label = lbl
		} else { // whole-map access: one column per discovered key
			header += wrappedColumn
			label = column
		}

		dataModel.addColumn(header, label, dt)
		dataModel.fillMapColumn(header, hdr, column, rows)
	}
}

// explodeMapForAggregateFunction explodes an aggregate applied to a map field, e.g.
// Avg(Quota('X')) or Avg(Properties). matches is the aggregateRe match of the header; dt is the
// map's element type, or nil for a plain MAP (per-key types from the :_ValueTypes sibling).
// The value copied per key is the aggregate's result, so it is taken as-is, without a per-row
// type check.
func (dataModel *DataModel) explodeMapForAggregateFunction(matches []string, lbl string, dt *string, rows []interface{}, cat *FunctionCatalog) {
	hdr := matches[0]
	aggregateFnc := matches[1]
	paramsRgx := regexp.MustCompile(`(?i)^distinct\s+`)
	paramsStr := paramsRgx.ReplaceAllString(matches[2], "")
	params := strings.Split(paramsStr, ",")
	// mapKey stays nil when the first argument is not a map access (e.g. a nested call);
	// "" means a whole-map access. The remaining arguments (a percentile, a bucket count, …)
	// are carried into each exploded column's name below.
	var mapField string
	var mapKey *string
	if field, key, ok := parseMapAccess(params[0], cat); ok {
		mapField = field
		mapKey = &key
	} else {
		mapField = params[0]
		mapKey = nil
	}
	shiftedParams := params[1:]

	var columnMap map[string][]string
	if dt == nil {
		columnMap = dataModel.scanMixedMapKeys(hdr, rows)
	} else {
		columnMap = dataModel.scanTypedMapKeys(hdr, *dt, rows)
	}

	// No row carried data for the aggregate's key: still emit the column, defaulting its type
	// to DECIMAL — aggregates are mostly math functions, so a number is the likeliest type.
	if len(columnMap) == 0 && mapKey != nil {
		columnMap[*mapKey] = []string{DECIMAL}
	}

	for _, column := range slices.Sorted(maps.Keys(columnMap)) {
		types := columnMap[column]
		addTypeToLabel := len(types) > 1
		for _, typ := range types {
			var otherParams string
			if len(shiftedParams) > 0 {
				otherParams = "," + strings.Join(shiftedParams, ",")
			}
			name := aggregateFnc + "(" + mapField + "('" + column + "')" + otherParams + ")"
			header := ConvertDtToPrefix(typ) + ":" + name
			// When the aggregate names one key (Avg(Quota('X'))) we keep the server's label, which is
			// the alias when the user gave one. When it aggregates a whole map (Avg(Quota), mapKey ""),
			// or a nested arg we could not reduce to a single key (mapKey nil), it explodes into one
			// column per key. Label each column with the aggregate wrapped around its key but without
			// the map-field name, e.g. Avg(Quota) key "MsgsPerDay" -> "Avg(MsgsPerDay)". Keeping the
			// aggregate keeps Avg vs Sum distinct when several are queried; dropping the field name
			// keeps it short. Any alias on the whole map is intentionally ignored here: one alias
			// cannot label the several columns the map explodes into.
			label := lbl
			if mapKey == nil || *mapKey == "" {
				label = aggregateFnc + "(" + column + otherParams + ")"
			}
			if addTypeToLabel {
				label += " (" + CapitalizeStr(typ) + ")"
			}

			dataModel.addColumn(header, label, typ)
			dataModel.fillMapColumn(header, hdr, column, rows)
		}
	}
}

// explodeMixedMapForFunction explodes a non-aggregate function result that is a plain MAP, e.g.
// Length(Properties('a')) or Concat(Statistics). The naming differs from explodeMixedMap: the key
// is matched anywhere inside the header (the function wraps the map access), and exploded columns
// are labeled with the full name, not just the key. Values are copied raw.
func (dataModel *DataModel) explodeMixedMapForFunction(hdr string, lbl string, rows []interface{}) {
	columnMap := dataModel.scanMixedMapKeys(hdr, rows)

	// No row carried data: still emit one column (keyed by the header itself), defaulting to
	// STRING — the default type for anything that isn't an aggregate result.
	if len(columnMap) == 0 {
		columnMap[hdr] = []string{STRING}
	}

	for _, column := range slices.Sorted(maps.Keys(columnMap)) {
		types := columnMap[column]
		addTypeToLabel := len(types) > 1
		for _, typ := range types {
			name := hdr
			header := ConvertDtToPrefix(typ) + ":" + name
			var label string
			wrappedColumn := "('" + column + "')"
			if hdr == column || strings.Contains(hdr, wrappedColumn) { // key already named in the call
				label = lbl
			} else { // whole-map result: one column per discovered key
				header += wrappedColumn
				name += wrappedColumn
				label = name
				if addTypeToLabel {
					label += " (" + CapitalizeStr(typ) + ")"
				}
			}

			dataModel.addColumn(header, label, typ)
			dataModel.fillMapColumn(header, hdr, column, rows)
		}
	}
}

// explodeTypedMapForFunction is explodeMixedMapForFunction for a typed map result MAP(dt): same
// naming policy, single element type, values copied without a per-row type check.
func (dataModel *DataModel) explodeTypedMapForFunction(hdr string, lbl string, dt string, rows []interface{}) {
	columnMap := dataModel.scanTypedMapKeys(hdr, dt, rows)

	for _, column := range slices.Sorted(maps.Keys(columnMap)) {
		name := hdr
		header := ConvertDtToPrefix(dt) + ":" + name
		var label string
		wrappedColumn := "('" + column + "')"
		if hdr == column || strings.Contains(hdr, wrappedColumn) { // key already named in the call
			label = lbl
		} else { // whole-map result: one column per discovered key
			header += wrappedColumn
			name += wrappedColumn
			label = name
		}

		dataModel.addColumn(header, label, dt)
		dataModel.fillMapColumn(header, hdr, column, rows)
	}
}

// BuildDataModel converts a parsed dataservice result set into the column-oriented DataModel
// used to build Grafana data frames. cat is the function catalog used to recognize
// function/aggregate headers; pass nil to use the built-in default. A shape violation is never
// fatal: the offending value/row/column becomes null or is skipped, the rest of the result
// survives, and the violation is recorded in model.Issues for the caller to log.
func BuildDataModel(parsedResultSet map[string]interface{}, cat *FunctionCatalog) DataModel {
	if cat == nil {
		cat = defaultFunctionCatalog
	}
	model := newDataModel(parsedResultSet)

	rows, rowsOk := parsedResultSet["rows"].([]interface{})
	if model.RowCount > 0 && !rowsOk {
		model.Issues.Add(`"rows" is missing or not an array`)
	}
	if model.RowCount > 0 && rowsOk {
		for _, col := range model.resultColumns(parsedResultSet) {
			model.buildColumn(col, rows, cat)
		}
	}

	// A query can produce the same exploded column twice — a whole map and one of its named keys
	// (Properties, Properties('a')) both explode to the same header. The later pass overwrote the
	// earlier one's metadata and row values with the same data, so only the repeated Headers
	// entries need dropping.
	model.Headers = dedupeStrings(model.Headers)

	return model
}

// newDataModel builds the empty model from the result-set envelope: row counts, in-body status,
// with RowCount empty rows ready to be filled column by column.
func newDataModel(parsedResultSet map[string]interface{}) DataModel {
	rowCount64, _ := ToInt64(parsedResultSet["row-count"])
	totalRowCount64, _ := ToInt64(parsedResultSet["total-row-count"])

	// In-body result status (distinct from the HTTP 400 / jk_ccode error envelope): the server
	// reports SUCCESS/WARNING/ERROR plus an optional message (e.g. row-limit truncation).
	status, _ := parsedResultSet["status"].(string)
	statusMsg, _ := parsedResultSet["status-msg"].(string)

	model := DataModel{
		RowCount:      int(rowCount64),
		TotalRowCount: int(totalRowCount64),
		Status:        status,
		StatusMsg:     statusMsg,
		Headers:       make([]string, 0),
		Label:         make(map[string]string),
		DataTypes:     make(map[string]string),
		Rows:          make([]map[string]interface{}, int(rowCount64)),
		Issues:        &ParseIssues{},
	}
	for index := range model.Rows {
		model.Rows[index] = make(map[string]interface{})
	}
	return model
}

// resultColumn is one column of the wire result set: its raw header, its jKQL data type and its
// display label, as read from the colhdr/coltype/collabel envelope entries.
type resultColumn struct {
	hdr   string
	typ   string
	label string
}

// resultColumns reads the result set's column metadata, skipping (and recording) malformed
// entries and the internal Solr SCORE column. A missing label falls back to the header.
func (dataModel *DataModel) resultColumns(parsedResultSet map[string]interface{}) []resultColumn {
	colHeaders, ok := parsedResultSet["colhdr"].([]interface{})
	if !ok {
		dataModel.Issues.Add(`"colhdr" is missing or not an array`)
	}
	colTypes, ok := parsedResultSet["coltype"].(map[string]interface{})
	if !ok {
		dataModel.Issues.Add(`"coltype" is missing or not an object`)
		colHeaders = nil // without types no column can be built
	}
	colLabels, ok := parsedResultSet["collabel"].(map[string]interface{})
	if !ok {
		dataModel.Issues.Add(`"collabel" is missing or not an object`)
	}

	columns := make([]resultColumn, 0, len(colHeaders))
	for _, h := range colHeaders {
		hdr, ok := h.(string)
		if !ok {
			dataModel.Issues.Add("column header is not a string")
			continue
		}
		typ, ok := colTypes[hdr].(string)
		if !ok {
			dataModel.Issues.Add("column " + hdr + ": type is missing or not a string; column skipped")
			continue
		}
		label, ok := colLabels[hdr].(string)
		if !ok {
			dataModel.Issues.Add("column " + hdr + ": label is missing or not a string")
			label = hdr
		}
		if hdr == SCORE {
			continue // Solr Score field, skip it
		}
		columns = append(columns, resultColumn{hdr: hdr, typ: typ, label: label})
	}
	return columns
}

// buildColumn converts one wire column into its model column(s). It dispatches first on the
// header's shape — a Properties access, an aggregate over a map — and then on the column's
// jKQL data type.
func (dataModel *DataModel) buildColumn(col resultColumn, rows []interface{}, cat *FunctionCatalog) {
	hdr, typ, label := col.hdr, col.typ, col.label

	// Properties gets special logic: it is the one MAP field always exploded, and it also
	// appears as a scalar-typed single-key access.
	if strings.HasPrefix(hdr, PROPERTIES) {
		dataModel.buildPropertiesColumn(hdr, typ, label, rows, cat)
		return
	}

	// An aggregate function applied to a MAP field (Avg(Quota('X')), Sum(Properties), ...).
	if matches := cat.aggregateRe.FindStringSubmatch(hdr); matches != nil {
		switch typ {
		case MAP:
			dataModel.explodeMapForAggregateFunction(matches, label, nil, rows, cat)
			return
		case MAP_INTEGER, MAP_DECIMAL, MAP_TIMEINTERVAL, MAP_TIMESTAMP, MAP_STRING, MAP_BOOLEAN:
			convertedTyp := ConvertDtMapToSimple(typ)
			dataModel.explodeMapForAggregateFunction(matches, label, &convertedTyp, rows, cat)
			return
		}
	}

	// A non-aggregate function applied to a MAP field (Length(Properties('a')), Concat(Statistics)).
	if cat.functionRe.MatchString(hdr) {
		switch typ {
		case MAP:
			dataModel.explodeMixedMapForFunction(hdr, label, rows)
			return
		case MAP_INTEGER, MAP_DECIMAL, MAP_TIMEINTERVAL, MAP_TIMESTAMP, MAP_STRING, MAP_BOOLEAN:
			dataModel.explodeTypedMapForFunction(hdr, label, ConvertDtMapToSimple(typ), rows)
			return
		}
	}

	switch typ {
	case BINARY, BOOLEAN, INTEGER, DECIMAL, TIMEINTERVAL, TIMESTAMP, STRING, ENUM, CLOB:
		// CLOB (deprecated in v12 but still emitted, e.g. cast(x,'CLOB')) is text — treat as
		// STRING. ENUM is rendered as the raw "ordinal#name" wire value for now.
		dataModel.addRawColumn(hdr, typ, label, rows)

	case STRING_ARR, BOOLEAN_ARR, DECIMAL_ARR, INTEGER_ARR, TIMEINTERVAL_ARR, TIMESTAMP_ARR, CLOB_ARR, BINARY_ARR:
		dataModel.addArrayColumn(hdr, typ, label, rows)

	case MAP, MAP_INTEGER, MAP_DECIMAL, MAP_TIMEINTERVAL, MAP_TIMESTAMP, MAP_STRING, MAP_BOOLEAN:
		// A bare MAP field besides Properties (Statistics, Objectives, …), not wrapped in an
		// aggregate or other function call (those are handled above): no explode policy for it,
		// renders as a single JSON column instead.
		dataModel.addRawColumn(hdr, typ, label, rows)

	default:
		// Any coltype not enumerated above (arrays, unknown types) is rendered as a raw column
		// for now, so it isn't silently dropped from the frame.
		switch {
		case strings.HasSuffix(typ, "[]"):
			dataModel.addArrayColumn(hdr, typ, label, rows)
		default:
			dataModel.addRawColumn(hdr, typ, label, rows)
		}
	}
}

// propertiesScalarRe matches a single-key Properties access, Properties('key') — the only header
// shape a scalar-typed Properties column can have.
var propertiesScalarRe = regexp.MustCompile(`(?i)^Properties\('(.+)'\)$`)

// buildPropertiesColumn handles a Properties column: the whole map (or a multi-key access)
// explodes into one column per key; a scalar sub-value (a Properties('key') access with a
// concrete type) becomes a single type-prefixed column. Any other coltype drops the column,
// recorded as an issue.
func (dataModel *DataModel) buildPropertiesColumn(hdr, typ, label string, rows []interface{}, cat *FunctionCatalog) {
	switch typ {
	case MAP:
		dataModel.explodeMixedMap(hdr, label, rows, cat)

	case BOOLEAN, INTEGER, DECIMAL, TIMESTAMP, STRING, TIMEINTERVAL, CLOB, ENUM:
		if !propertiesScalarRe.MatchString(hdr) {
			dataModel.Issues.Add("column " + hdr + ": not a Properties('key') access; column skipped")
			return
		}
		// The header gets a type prefix (I:Properties('x')) so the same key requested with two
		// types (via casts) stays two distinct columns, like an exploded mixed-map key.
		header := ConvertDtToPrefix(typ) + ":" + hdr
		dataModel.addColumn(header, label, typ)
		for index, r := range rows {
			dataModel.Rows[index][header] = nil
			row, ok := dataModel.rowObject(r, hdr)
			if !ok {
				continue
			}
			dataModel.Rows[index][header] = row[hdr]
		}

	default:
		dataModel.Issues.Add("column " + hdr + ": unsupported Properties coltype " + typ + "; column skipped")
	}
}
