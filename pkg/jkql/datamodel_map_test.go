package jkql

import (
	"reflect"
	"sort"
	"testing"
)

func TestParseMapAccess(t *testing.T) {
	cat := DefaultFunctionCatalog()
	cases := []struct {
		in        string
		wantField string
		wantKey   string
		wantOK    bool
	}{
		// map-field accesses
		{"Properties('Timezone')", "Properties", "Timezone", true},
		{"Statistics('MyStat')", "Statistics", "MyStat", true},
		{"Dimensions('region')", "Dimensions", "region", true}, // unknown map field still parses
		{"Properties", "Properties", "", true},                 // bare field = whole map, empty key
		// multi-key: the raw key blob is returned; namedMapKeys splits it
		{"Properties('A','B')", "Properties", "A','B", true},
		// known functions share the Field('key') shape but must NOT be treated as map access
		{"Round('x')", "", "", false},
		{"Avg('x')", "", "", false},
		// a function call without a quoted key isn't a map access either
		{"Round(x)", "", "", false},
	}
	for _, c := range cases {
		field, key, ok := parseMapAccess(c.in, cat)
		if field != c.wantField || key != c.wantKey || ok != c.wantOK {
			t.Errorf("parseMapAccess(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.in, field, key, ok, c.wantField, c.wantKey, c.wantOK)
		}
	}
}

func TestNamedMapKeys(t *testing.T) {
	cat := DefaultFunctionCatalog()
	cases := []struct {
		in   string
		want []string
	}{
		{"Properties('Timezone')", []string{"Timezone"}},
		{"Properties('A','B','C')", []string{"A", "B", "C"}},
		{"Properties('A', 'B')", []string{"A", "B"}}, // tolerates spaces between keys
		{"Properties", nil}, // whole map names no keys
		{"Round('x')", nil}, // function, not a map access
	}
	for _, c := range cases {
		got := namedMapKeys(c.in, cat)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("namedMapKeys(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func labelsOf(m DataModel) []string {
	out := make([]string, 0, len(m.Headers))
	for _, h := range m.Headers {
		out = append(out, m.Label[h])
	}
	sort.Strings(out)
	return out
}

func TestBuildDataModel_NamedPropertyNullStillYieldsColumn(t *testing.T) {
	// Properties('Timezone') comes back as a MAP that is null on every row (unset, or mistyped).
	// The column must still appear (named access always yields its column), labeled by the key.
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Properties('Timezone')"],
		"coltype": {"Properties('Timezone')": "MAP"},
		"collabel": {"Properties('Timezone')": "Timezone"},
		"rows": [ {"Properties('Timezone')": null} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 1 {
		t.Fatalf("want 1 column, got %d: %v", len(m.Headers), m.Headers)
	}
	if got := m.Label[m.Headers[0]]; got != "Timezone" {
		t.Errorf("label = %q, want %q", got, "Timezone")
	}
}

func TestBuildDataModel_NamedPropertyReadsValue(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Properties('Timezone')"],
		"coltype": {"Properties('Timezone')": "MAP"},
		"collabel": {"Properties('Timezone')": "Timezone"},
		"rows": [ {
			"Properties('Timezone')": {"Timezone": "UTC"},
			"Properties('Timezone'):_ValueTypes": {"Timezone": "STRING"}
		} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 1 {
		t.Fatalf("want 1 column, got %d: %v", len(m.Headers), m.Headers)
	}
	header := m.Headers[0]
	if m.Label[header] != "Timezone" {
		t.Errorf("label = %q, want %q", m.Label[header], "Timezone")
	}
	if m.Rows[0][header] != "UTC" {
		t.Errorf("value = %v, want UTC", m.Rows[0][header])
	}
}

func TestBuildDataModel_WholeMapExplodesPerKey(t *testing.T) {
	// Properties (whole map) explodes into one column per key found in the data, labeled by the
	// key.
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Properties"],
		"coltype": {"Properties": "MAP"},
		"collabel": {"Properties": "Properties"},
		"rows": [ {
			"Properties": {"UserName": "Admin", "Host": "h1"},
			"Properties:_ValueTypes": {"UserName": "STRING", "Host": "STRING"}
		} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	want := []string{"Host", "UserName"}
	if got := labelsOf(m); !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

func TestBuildDataModel_MixedMapKeyWithTwoTypesGetsTypeSuffix(t *testing.T) {
	// The same key holds a STRING on one row and an INTEGER on another: it explodes into two
	// columns, one per type, each labeled with a " (Type)" suffix to disambiguate them.
	raw := `{
		"row-count": 2, "total-row-count": 2, "status": "SUCCESS",
		"colhdr": ["Properties"],
		"coltype": {"Properties": "MAP"},
		"collabel": {"Properties": "Properties"},
		"rows": [
			{"Properties": {"Retries": "none"}, "Properties:_ValueTypes": {"Retries": "STRING"}},
			{"Properties": {"Retries": 3}, "Properties:_ValueTypes": {"Retries": "INTEGER"}}
		]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	want := []string{"Retries (Integer)", "Retries (String)"}
	if got := labelsOf(m); !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

func TestBuildDataModel_AggregateOverMapLabels(t *testing.T) {
	// Avg over a whole map explodes per key; each column keeps the aggregate and drops the field
	// name (Avg(Quota) -> "Avg(<key>)"), so Avg vs Sum stay distinct when both are queried.
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Avg(Quota)"],
		"coltype": {"Avg(Quota)": "MAP(DECIMAL)"},
		"collabel": {"Avg(Quota)": "Avg(Quota)"},
		"rows": [ {
			"Avg(Quota)": {"MsgsPerDay": 10.0, "BytesPerDay": 20.0}
		} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	want := []string{"Avg(BytesPerDay)", "Avg(MsgsPerDay)"}
	if got := labelsOf(m); !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

func TestBuildDataModel_AggregateOverNamedMapKeyKeepsLabel(t *testing.T) {
	// Avg(Quota('MsgsPerDay')) names one key: the server's label (or alias) is kept as-is,
	// instead of being rewritten to "Avg(MsgsPerDay)".
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Avg(Quota('MsgsPerDay'))"],
		"coltype": {"Avg(Quota('MsgsPerDay'))": "MAP(DECIMAL)"},
		"collabel": {"Avg(Quota('MsgsPerDay'))": "Average Daily Messages"},
		"rows": [ {
			"Avg(Quota('MsgsPerDay'))": {"MsgsPerDay": 10.0}
		} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	want := []string{"Average Daily Messages"}
	if got := labelsOf(m); !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

func TestBuildDataModel_UnnamedMapFieldNotExploded(t *testing.T) {
	// A MAP field other than Properties (no explode policy) renders as a single raw column,
	// not exploded per key.
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Statistics"],
		"coltype": {"Statistics": "MAP"},
		"collabel": {"Statistics": "Statistics"},
		"rows": [ {
			"Statistics": {"A": 1},
			"Statistics:_ValueTypes": {"A": "INTEGER"}
		} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 1 || m.Headers[0] != "Statistics" {
		t.Errorf("headers = %v, want just [Statistics] kept whole", m.Headers)
	}
}

func TestBuildDataModel_PropertiesScalarAccess(t *testing.T) {
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Properties('Region')"],
		"coltype": {"Properties('Region')": "STRING"},
		"collabel": {"Properties('Region')": "Region"},
		"rows": [ {"Properties('Region')": "us-east"} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 1 {
		t.Fatalf("want 1 column, got %d: %v", len(m.Headers), m.Headers)
	}
	header := m.Headers[0]
	if m.Label[header] != "Region" {
		t.Errorf("label = %q, want Region", m.Label[header])
	}
	if m.Rows[0][header] != "us-east" {
		t.Errorf("value = %v, want us-east", m.Rows[0][header])
	}
}

func TestBuildDataModel_PropertiesNonKeyedScalarDropped(t *testing.T) {
	// A scalar-typed Properties column that isn't a Properties('key') access shouldn't happen on
	// the wire, but if it did, it must be dropped with a recorded issue rather than misread.
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Properties"],
		"coltype": {"Properties": "STRING"},
		"collabel": {"Properties": "Properties"},
		"rows": [ {"Properties": "oops"} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	if len(m.Headers) != 0 {
		t.Errorf("headers = %v, want none (dropped)", m.Headers)
	}
	if len(m.Issues.List()) == 0 {
		t.Error("a non-keyed scalar Properties column should record a parse issue")
	}
}

func TestBuildDataModel_NonAggregateFunctionOverMapExplodesPerKey(t *testing.T) {
	// A non-aggregate function (Length is scalar, not aggregate) applied to a whole map explodes
	// per key, same as an aggregate would, but keeps the full call in the label instead of
	// dropping the field name.
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Length(Statistics)"],
		"coltype": {"Length(Statistics)": "MAP"},
		"collabel": {"Length(Statistics)": "Length(Statistics)"},
		"rows": [ {
			"Length(Statistics)": {"Name": 4},
			"Length(Statistics):_ValueTypes": {"Name": "INTEGER"}
		} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	want := []string{"Length(Statistics)('Name')"}
	if got := labelsOf(m); !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

func TestBuildDataModel_NonAggregateFunctionOverNamedMapKeyKeepsLabel(t *testing.T) {
	// Length(Properties('Region')) names one key: the server's label is kept as-is.
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Length(Properties('Region'))"],
		"coltype": {"Length(Properties('Region'))": "MAP(INTEGER)"},
		"collabel": {"Length(Properties('Region'))": "Region Length"},
		"rows": [ {
			"Length(Properties('Region'))": {"Region": 7}
		} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	want := []string{"Region Length"}
	if got := labelsOf(m); !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

func TestBuildDataModel_WholeMapPlusNamedKey_NoDuplicateColumn(t *testing.T) {
	// A whole map and one of its named keys explode to the same header; it must appear once,
	// or the frame gets two identical fields.
	raw := `{
		"row-count": 1, "total-row-count": 1, "status": "SUCCESS",
		"colhdr": ["Properties", "Properties('a')"],
		"coltype": {"Properties": "MAP", "Properties('a')": "MAP"},
		"collabel": {"Properties": "Properties", "Properties('a')": "a"},
		"rows": [ {
			"Properties": {"a": 1.5},
			"Properties:_ValueTypes": {"a": "DECIMAL"},
			"Properties('a')": {"a": 1.5},
			"Properties('a'):_ValueTypes": {"a": "DECIMAL"}
		} ]
	}`
	m := BuildDataModel(parseRS(t, raw), nil)

	want := []string{"D:Properties('a')"}
	if !reflect.DeepEqual(m.Headers, want) {
		t.Fatalf("headers = %v, want %v", m.Headers, want)
	}
}
