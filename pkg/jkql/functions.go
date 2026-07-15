package jkql

import (
	"regexp"
	"strings"
)

// jKQL function catalog. Names are grouped by the server's function categories (Aggregate /
// Analytic / Scalar). The server normalizes every alias to its one canonical name in the header
// (Len -> Length, Ucase -> Upper, StdDev -> StdDevPop), so only canonical names are listed. Within
// each group the v11-only names are marked, so they can be removed in one step when v11 support is
// dropped (v12 is a strict subset of v11). This is a hardcoded list for now; loading it from the
// server is a later step.

var aggregateFunctions = []string{
	"Apdex", "Avg", "Close", "Count", "List", "Max", "Median", "Min", "Open",
	"Percentile", "StdDevPop", "StdDevSample", "Sum", "VariancePop", "VarianceSample",
	// (no v11-only aggregates)
}

var analyticFunctions = []string{
	"AllPathsBetween", "AllPathsToFrom", "BBands", "ExpMovingAvg", "Extrapolate", "ForEach",
	"HoltWintersPrediction", "MaximumFlow", "RogueEdges", "ShortestPath", "SimpleMovingAvg",
	// v11 only (remove when v11 support is dropped):
	"Anomaly", "ClusterDetails", "Clusters", "Clusters3d", "Clusters3dMultiKey", "ClustersMultiKey",
	"Correlate", "ExpAnomaliesInRange", "Expected", "FCAnomaliesInRange", "FCAnomaliesPriorPeriod",
	"FeatureSelection", "FeatureSuggestion", "FeatureSuggestionPriorPeriod", "Forecast", "FsugNum",
	"MultiAnomaliesPriorPeriod", "RegAPP", "WhatIf",
}

var scalarFunctions = []string{
	"Abs", "AvgOf", "Cast", "Ceil", "Coalesce", "Concat", "ConcatWS", "DateAdd", "DateAdjust",
	"DateDiff", "DateExtract", "DayOfWeek", "Exp", "FindIn", "Floor", "Left", "Length", "Ln",
	"Log10", "Lower", "MaxOf", "MedianOf", "MinOf", "Now", "Position", "PositionRE", "Power",
	"Replace", "Right", "Round", "Sqrt", "StrAt", "SubStr", "SubStrRE", "SumOf", "Tokenize",
	"UUID", "Upper", "ValueAt",
	// v11 only (remove when v11 support is dropped):
	"Delta", "Next", "PercentChg", "Previous",
}

// FunctionCatalog recognizes jKQL function names in result-set column headers. It exists so
// parseMapAccess can tell a map-field access like Properties('key') apart from a same-shaped
// function call like Round('x'). The converter never loads it; the caller passes one in, or nil
// for the hardcoded default below.
type FunctionCatalog struct {
	// aggregateRe matches an aggregate header, capturing the name and inner args:
	// "Avg(Quota('x'))" -> ["Avg(Quota('x'))", "Avg", "Quota('x')"].
	aggregateRe *regexp.Regexp
	// functionRe matches a non-aggregate (analytic or scalar) function header. Aggregates are
	// matched by aggregateRe first, so they are excluded here. Example: "Length(Message)" matches.
	functionRe *regexp.Regexp
	// names is every known function name (all three groups). It lets parseMapAccess tell a field
	// access Field('key') apart from a same-shaped function call Func('arg').
	names map[string]bool
}

// NewFunctionCatalog builds a catalog from the three function-name groups — the categories the
// server's function set is organized into (Aggregate / Analytic / Scalar).
func NewFunctionCatalog(aggregate, analytic, scalar []string) *FunctionCatalog {
	return &FunctionCatalog{
		aggregateRe: regexp.MustCompile(`^(` + orNames(aggregate) + `)\((.+)\)$`),
		functionRe:  regexp.MustCompile(`^(?:` + orNames(analytic, scalar) + `)\(.+\)$`),
		names:       nameSet(aggregate, analytic, scalar),
	}
}

// defaultFunctionCatalog is the built-in fallback, from the hardcoded canonical lists above.
var defaultFunctionCatalog = NewFunctionCatalog(aggregateFunctions, analyticFunctions, scalarFunctions)

// DefaultFunctionCatalog returns the built-in fallback catalog.
func DefaultFunctionCatalog() *FunctionCatalog { return defaultFunctionCatalog }

// noNameMatches is a regex fragment that can never match anything (Go's RE2 engine has no
// lookahead, so this — a literal followed by an impossible position, string-start after a
// consumed character — is the standard RE2-safe way to build one). Used when every name group is
// empty: an empty alternation "()" would otherwise match any header, e.g. one whole-catalog
// group (aggregate, or analytic+scalar together) legitimately having zero names would make its
// regex match everything instead of nothing.
const noNameMatches = `a^`

// orNames joins the given name groups into a regex alternation (a|b|c). Empty names are skipped,
// so a stray blank entry in a group can't turn into an empty alternative that matches at any
// position in the caller's regex; if every group is empty, returns noNameMatches instead of an
// empty string, so the caller's regex can't degenerate into one that matches any input.
func orNames(groups ...[]string) string {
	var all []string
	for _, g := range groups {
		for _, n := range g {
			if n != "" {
				all = append(all, n)
			}
		}
	}
	if len(all) == 0 {
		return noNameMatches
	}
	return strings.Join(all, "|")
}

// nameSet collects the given name groups into a lookup set.
func nameSet(groups ...[]string) map[string]bool {
	set := make(map[string]bool)
	for _, g := range groups {
		for _, n := range g {
			set[n] = true
		}
	}
	return set
}
