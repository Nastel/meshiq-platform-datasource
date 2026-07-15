package jkql

// This file holds the jKQL data-type system: the column data-type names and the field-name
// constants the converter special-cases.

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

	UNDEFINED = ""
)

// FieldType is a jKQL field name. Only the field names the code special-cases are listed here;
// every other field name is handled generically by its string value from the result set.
type FieldType = string

const (
	SCORE FieldType = "Score" // Solr score field, skipped

	// metadata query columns (see the "Get Params" health check)
	NAME FieldType = "Name"
)
