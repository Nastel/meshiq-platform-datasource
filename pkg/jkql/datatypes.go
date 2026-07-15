package jkql

// This file holds the jKQL data-type system: the column data-type names and the field-name
// constants the converter special-cases.

// SUFIX_VALUETYPES is the suffix of a MAP column's sibling that reports each key's value type,
// e.g. "Properties:_ValueTypes" alongside "Properties".
const SUFIX_VALUETYPES string = ":_ValueTypes"

// DataType enumerates the jKQL column data types.
type DataType = string

const (
	// simple data types
	DECIMAL      = "DECIMAL"
	INTEGER      = "INTEGER"
	STRING       = "STRING"
	TIMEINTERVAL = "TIMEINTERVAL"
	TIMESTAMP    = "TIMESTAMP"
	BOOLEAN      = "BOOLEAN"
	ENUM         = "ENUM"
	BINARY       = "BINARY"
	CLOB         = "CLOB"

	// array data types
	BOOLEAN_ARR      = "BOOLEAN[]"
	DECIMAL_ARR      = "DECIMAL[]"
	INTEGER_ARR      = "INTEGER[]"
	STRING_ARR       = "STRING[]"
	TIMEINTERVAL_ARR = "TIMEINTERVAL[]"
	TIMESTAMP_ARR    = "TIMESTAMP[]"
	CLOB_ARR         = "CLOB[]"
	BINARY_ARR       = "BINARY[]"

	// map data types
	MAP              = "MAP"
	MAP_BOOLEAN      = "MAP(BOOLEAN)"
	MAP_DECIMAL      = "MAP(DECIMAL)"
	MAP_INTEGER      = "MAP(INTEGER)"
	MAP_STRING       = "MAP(STRING)"
	MAP_TIMEINTERVAL = "MAP(TIMEINTERVAL)"
	MAP_TIMESTAMP    = "MAP(TIMESTAMP)"

	UNDEFINED = ""
)

// FieldType is a jKQL field name. Only the field names the code special-cases are listed here;
// every other field name is handled generically by its string value from the result set.
type FieldType = string

const (
	PROPERTIES FieldType = "Properties" // exploded into one column per key (see datamodel.go)
	SCORE      FieldType = "Score"      // Solr score field, skipped

	// metadata query columns (see the "Get Params" health check)
	NAME FieldType = "Name"
)

// ConvertDtToPrefix returns the single-letter data-type prefix jKQL uses in column headers for an
// exploded map key (so the same key requested with two types, e.g. via casts, stays two distinct
// columns).
func ConvertDtToPrefix(dataType string) string {
	switch dataType {
	case BOOLEAN:
		return "B"
	case INTEGER:
		return "I"
	case DECIMAL:
		return "D"
	case TIMESTAMP:
		return "T"
	case TIMEINTERVAL:
		return "V"
	case STRING:
		return "S"
	case ENUM:
		return "E"
	case BINARY:
		return "X"
	case CLOB:
		return "C"
	default:
		// An unknown type can reach here via a map's :_ValueTypes sibling, which carries
		// arbitrary server-side type names. No prefix is better than a wrong one.
		return ""
	}
}

// ConvertDtMapToSimple returns the element data type of a MAP(...) type.
func ConvertDtMapToSimple(dataType string) string {
	switch dataType {
	case MAP_BOOLEAN:
		return BOOLEAN
	case MAP_INTEGER:
		return INTEGER
	case MAP_DECIMAL:
		return DECIMAL
	case MAP_TIMESTAMP:
		return TIMESTAMP
	case MAP_TIMEINTERVAL:
		return TIMEINTERVAL
	case MAP_STRING:
		return STRING
	default:
		// should never happen
		return UNDEFINED
	}
}
