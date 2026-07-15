package jkql

// addColumn registers one output column's metadata: its header, display label and jKQL data
// type. Row values are filled separately by the caller.
func (dataModel *DataModel) addColumn(header, label, typ string) {
	dataModel.Headers = append(dataModel.Headers, header)
	dataModel.Label[header] = label
	dataModel.DataTypes[header] = typ
}

// addRawColumn adds a field as a single column holding each row's value as-is. Used for plain
// scalar columns.
func (dataModel *DataModel) addRawColumn(hdr string, typ string, lbl string, rows []interface{}) {
	dataModel.addColumn(hdr, lbl, typ)

	for i, r := range rows {
		row, ok := r.(map[string]interface{})
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
		row, ok := r.(map[string]interface{})
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

// BuildDataModel converts a parsed dataservice result set into the column-oriented DataModel
// used to build Grafana data frames. model.Issues is plumbed through every builder function
// (see ParseIssues) but nothing populates it yet.
func BuildDataModel(parsedResultSet map[string]interface{}) DataModel {
	model := newDataModel(parsedResultSet)

	rows, _ := parsedResultSet["rows"].([]interface{})
	if model.RowCount > 0 {
		for _, col := range model.resultColumns(parsedResultSet) {
			model.buildColumn(col, rows)
		}
	}

	return model
}

// maxRowCountOverhead caps how far a declared "row-count" is allowed to exceed the actual rows
// array. The server never legitimately reports more rows than it sends in the same response; this
// only guards against a garbage or corrupted count, which would otherwise size Rows off a huge
// number and exhaust memory before a single row is ever read.
const maxRowCountOverhead = 100000

// newDataModel builds the empty model from the result-set envelope: row counts, in-body status,
// with RowCount empty rows ready to be filled column by column. The declared row-count is
// reconciled with the actual rows array first: the fill loops iterate the array and index Rows
// by position, so a count that is negative or smaller than len(rows) would panic — the array
// wins in both cases and the mismatch is recorded. A count larger than len(rows) keeps its
// existing meaning (the missing tail rows stay null), but is clamped to maxRowCountOverhead past
// len(rows) so a garbage count can't drive a runaway allocation.
func newDataModel(parsedResultSet map[string]interface{}) DataModel {
	rowCount64, _ := ToInt64(parsedResultSet["row-count"])
	totalRowCount64, _ := ToInt64(parsedResultSet["total-row-count"])

	issues := &ParseIssues{}
	rowCount := int(rowCount64)
	if rowCount < 0 {
		issues.Add(`"row-count" is negative`)
		rowCount = 0
	}
	actualRows, _ := parsedResultSet["rows"].([]interface{})
	if len(actualRows) > rowCount {
		issues.Add(`"row-count" is smaller than the number of rows sent`)
		rowCount = len(actualRows)
	}
	if maxRowCount := len(actualRows) + maxRowCountOverhead; rowCount > maxRowCount {
		issues.Add(`"row-count" is far larger than the number of rows sent, clamped`)
		rowCount = maxRowCount
	}

	// In-body result status (distinct from the HTTP 400 / jk_ccode error envelope): the server
	// reports SUCCESS/WARNING/ERROR plus an optional message (e.g. row-limit truncation).
	status, _ := parsedResultSet["status"].(string)
	statusMsg, _ := parsedResultSet["status-msg"].(string)

	model := DataModel{
		RowCount:      rowCount,
		TotalRowCount: int(totalRowCount64),
		Status:        status,
		StatusMsg:     statusMsg,
		Headers:       make([]string, 0),
		Label:         make(map[string]string),
		DataTypes:     make(map[string]string),
		Rows:          make([]map[string]interface{}, rowCount),
		Issues:        issues,
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

// resultColumns reads the result set's column metadata. A missing label falls back to the header.
func (dataModel *DataModel) resultColumns(parsedResultSet map[string]interface{}) []resultColumn {
	colHeaders, _ := parsedResultSet["colhdr"].([]interface{})
	colTypes, _ := parsedResultSet["coltype"].(map[string]interface{})
	colLabels, _ := parsedResultSet["collabel"].(map[string]interface{})

	columns := make([]resultColumn, 0, len(colHeaders))
	for _, h := range colHeaders {
		hdr, ok := h.(string)
		if !ok {
			continue
		}
		typ, ok := colTypes[hdr].(string)
		if !ok {
			continue
		}
		label, ok := colLabels[hdr].(string)
		if !ok {
			label = hdr
		}
		if hdr == SCORE {
			continue // Solr Score field, skip it
		}
		columns = append(columns, resultColumn{hdr: hdr, typ: typ, label: label})
	}
	return columns
}

// buildColumn converts one wire column into its model column, dispatching on the column's jKQL
// data type.
func (dataModel *DataModel) buildColumn(col resultColumn, rows []interface{}) {
	hdr, typ, label := col.hdr, col.typ, col.label

	switch typ {
	case BINARY, BOOLEAN, INTEGER, DECIMAL, TIMEINTERVAL, TIMESTAMP, STRING, ENUM, CLOB:
		// CLOB (deprecated in v12 but still emitted, e.g. cast(x,'CLOB')) is text — treat as
		// STRING. ENUM is rendered as the raw "ordinal#name" wire value for now.
		dataModel.addRawColumn(hdr, typ, label, rows)

	case STRING_ARR, BOOLEAN_ARR, DECIMAL_ARR, INTEGER_ARR, TIMEINTERVAL_ARR, TIMESTAMP_ARR, CLOB_ARR, BINARY_ARR:
		dataModel.addArrayColumn(hdr, typ, label, rows)

	default:
		// Any coltype not enumerated above (maps, ranges, variants, …) is rendered as a raw
		// column for now, so it isn't silently dropped from the frame.
		dataModel.addRawColumn(hdr, typ, label, rows)
	}
}
