package jkql

import (
	"fmt"
	"strconv"
	"strings"
)

// This file holds the jKQL data-type system: the column data-type names, the field-name
// constants the converter special-cases, and the enum value encoding/decoding.

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
	VARIANT      = "VARIANT"
	CLOB         = "CLOB"

	// array data types
	ENUM_ARR         = "ENUM[]"
	BOOLEAN_ARR      = "BOOLEAN[]"
	DECIMAL_ARR      = "DECIMAL[]"
	INTEGER_ARR      = "INTEGER[]"
	STRING_ARR       = "STRING[]"
	TIMEINTERVAL_ARR = "TIMEINTERVAL[]"
	TIMESTAMP_ARR    = "TIMESTAMP[]"
	VARIANT_ARR      = "VARIANT[]"
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

	// range data types
	RANGE_DECIMAL      = "RANGE(DECIMAL)"
	RANGE_INTEGER      = "RANGE(INTEGER)"
	RANGE_TIMEINTERVAL = "RANGE(TIMEINTERVAL)"
	RANGE_TIMESTAMP    = "RANGE(TIMESTAMP)"

	// LABELSET is a scalar string — one label from a fixed set — and LABELSET[] is its array
	// form; RANGE_GENERIC is an untyped range whose endpoints render as strings.
	LABELSET      = "LABELSET"
	LABELSET_ARR  = "LABELSET[]"
	RANGE_GENERIC = "RANGE"

	UNDEFINED = ""
)

// FieldType is a jKQL field name. Only the field names the code special-cases are listed here;
// every other field name is handled generically by its string value from the result set.
type FieldType = string

const (
	PROPERTIES FieldType = "Properties" // exploded into one column per key (see datamodel.go)
	SCORE      FieldType = "Score"      // Solr score field, skipped

	// metadata query columns: the "Get Params" health check, the /repositories, /tables and
	// /fields resources, and the "GET ENUMERATION FOR <field>" dense enum value table.
	ID         FieldType = "ID"
	NAME       FieldType = "Name"
	REPO_ID    FieldType = "RepositoryID"
	ITEM_NAME  FieldType = "ItemName"
	FIELD_NAME FieldType = "FieldName"
	DATA_TYPE  FieldType = "DataType"

	// Map fields other than Properties: whole-map access (bare field name, no key) stays a single
	// raw JSON column instead of exploding one column per key. A named key still always unwraps/
	// explodes, same as Properties('key') — see neverExplodeMapFields in datamodel.go.
	QUOTA                    FieldType = "Quota"
	EFFECTIVE_QUOTAS         FieldType = "EffectiveQuotas" // ORGANIZATION — same key shape as Quota
	STATISTICS               FieldType = "Statistics"
	META_DATA                FieldType = "MetaData"
	WGS_CUSTOM_PROPERTIES    FieldType = "WgsCustomProperties"
	WGS_KAFKA_CONFIGS        FieldType = "WgsKafkaConfigs"       // ~100+ keys (full Kafka broker config)
	WGS_KAFKA_CONFIG_LOGGERS FieldType = "WgsKafkaConfigLoggers" // ~50+ keys (per-class logger levels)

	// Map fields that never explode, for ANY access shape — bare, single-key, or multi-key alike. A
	// named key does NOT override this, unlike the fields above — see alwaysRawMapFields in
	// datamodel.go.
	OBJECTIVES FieldType = "Objectives"
	RULES      FieldType = "Rules"   // Monitor
	OPTIONS    FieldType = "Options" // Monitor, ACCESS_TOKEN, VIEW, EXT_ITEM(_FIELD)
	LICENSE    FieldType = "License"
)

// JkqlEnum represents an enumerated jKQL value, encoded on the wire as "ordinal#name".
type JkqlEnum struct {
	Ordinal int    `json:"ordinal"`
	Name    string `json:"name"`
}

// ToEnumObject parses the wire "ordinal#name" encoding into a JkqlEnum, tolerantly: a value
// that doesn't follow the encoding keeps its whole text as the name (ordinal 0), so it still
// displays instead of breaking the column.
func ToEnumObject(value interface{}) JkqlEnum {
	e, _ := toEnumObjectChecked(value)
	return e
}

// toEnumObjectChecked is ToEnumObject reporting whether the value followed the "ordinal#name"
// encoding, so converter call sites can record a parse issue for logging.
func toEnumObjectChecked(value interface{}) (JkqlEnum, bool) {
	if value == nil {
		return JkqlEnum{}, true
	}
	valueStr := fmt.Sprint(value)
	parts := strings.SplitN(valueStr, "#", 2)
	if len(parts) != 2 {
		return JkqlEnum{Ordinal: 0, Name: valueStr}, false
	}
	ordinal, err := strconv.Atoi(parts[0])
	if err != nil {
		return JkqlEnum{Ordinal: 0, Name: valueStr}, false
	}
	return JkqlEnum{Ordinal: ordinal, Name: parts[1]}, true
}

// ConvertDtToPrefix returns the data-type prefix jKQL uses in column headers for an exploded map
// key (so the same key requested with two types, e.g. via casts, stays two distinct columns).
// Scalars get a single letter; the array counterpart of each scalar adds a trailing "A" (e.g.
// STRING -> "S", STRING[] -> "SA") so an array-typed key can't collide with its own scalar form
// or with a different array type on the same key.
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
	case VARIANT:
		return "A"
	case CLOB:
		return "C"
	case LABELSET:
		return "L"
	case BOOLEAN_ARR:
		return "BA"
	case INTEGER_ARR:
		return "IA"
	case DECIMAL_ARR:
		return "DA"
	case TIMESTAMP_ARR:
		return "TA"
	case TIMEINTERVAL_ARR:
		return "VA"
	case STRING_ARR:
		return "SA"
	case ENUM_ARR:
		return "EA"
	case BINARY_ARR:
		return "XA"
	case VARIANT_ARR:
		return "AA"
	case CLOB_ARR:
		return "CA"
	case LABELSET_ARR:
		return "LA"
	default:
		// An unknown type can reach here via a map's :_ValueTypes sibling, which carries
		// arbitrary server-side type names (e.g. a coltype the server introduces later). No
		// prefix is better than a wrong one — but note this means two DIFFERENT unrecognized
		// types on the same map key still collide (same empty prefix -> same header), silently
		// dropping one column. Add a case above for any new type as soon as it's seen live.
		return ""
	}
}

// ConvertRangeToDt returns the element data type of a RANGE(...) type.
func ConvertRangeToDt(dataType string) string {
	switch dataType {
	case RANGE_INTEGER:
		return INTEGER
	case RANGE_DECIMAL:
		return DECIMAL
	case RANGE_TIMESTAMP:
		return TIMESTAMP
	case RANGE_TIMEINTERVAL:
		return TIMEINTERVAL
	default:
		// A generic RANGE or an unknown RANGE(...) coltype has no element type;
		// the caller (explodeRange) falls back to STRING endpoints.
		return UNDEFINED
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
		// unreachable today: callers dispatch only on the MAP_* constants above
		return UNDEFINED
	}
}
