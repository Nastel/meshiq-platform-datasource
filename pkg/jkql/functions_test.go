package jkql

import (
	"regexp"
	"testing"
)

func TestOrNames(t *testing.T) {
	got := orNames([]string{"a", "b"}, []string{"c"})
	if got != "a|b|c" {
		t.Errorf("orNames = %q, want %q", got, "a|b|c")
	}
}

// TestOrNames_AllGroupsEmptyMatchesNothing pins that a fully empty name set produces a regex
// fragment that matches no input — an empty alternation "()" would instead match anything,
// which is what a group left legitimately empty (e.g. the server reports zero Aggregate
// functions while Analytic/Scalar are non-empty) used to turn into a match-everything regex.
func TestOrNames_AllGroupsEmptyMatchesNothing(t *testing.T) {
	for _, got := range []string{orNames(), orNames([]string{}), orNames([]string{}, []string{})} {
		re := regexp.MustCompile(`^(?:` + got + `)\(.+\)$`)
		if re.MatchString("(anything)") {
			t.Errorf("orNames of nothing = %q, produced a regex that matches an empty-name header", got)
		}
		if re.MatchString("SomeFunc(x)") {
			t.Errorf("orNames of nothing = %q, produced a regex that matches an unrelated header", got)
		}
	}
}

func TestNameSet(t *testing.T) {
	set := nameSet([]string{"a", "b"}, []string{"b", "c"})
	for _, n := range []string{"a", "b", "c"} {
		if !set[n] {
			t.Errorf("nameSet missing %q", n)
		}
	}
	if set["z"] {
		t.Error("nameSet should not contain 'z'")
	}
	if len(set) != 3 {
		t.Errorf("nameSet len = %d, want 3 (deduped)", len(set))
	}
}

// TestNewFunctionCatalog_EmptyNameSkipped pins that an empty name in a group is skipped, so it
// can't degenerate the alternation into one that matches at any position. A group holding "" (a
// stray/blank entry from the server's function set) must not make aggregateRe match a
// paren-leading header like "(abc)", while a real name in the same group still matches.
func TestNewFunctionCatalog_EmptyNameSkipped(t *testing.T) {
	cat := NewFunctionCatalog(
		[]string{"Avg", ""}, // aggregate group carries a stray empty name
		[]string{},
		[]string{},
	)

	if cat.aggregateRe.MatchString("(abc)") {
		t.Error("aggregateRe must not match a paren-leading header when a group holds an empty name")
	}
	if !cat.aggregateRe.MatchString("Avg(abc)") {
		t.Error("aggregateRe should still match the real aggregate name Avg")
	}
}

// TestNewFunctionCatalog_EmptyAggregateGroupMatchesNoAggregateHeader pins the real-world case:
// the server's "get functions" reports real Analytic/Scalar names but zero Aggregate ones (a
// plausible partial response, not the total-failure case that already falls back to the built-in
// default). aggregateRe is built from the Aggregate group alone (see NewFunctionCatalog), so it
// must not become a match-everything regex just because that one group came back empty.
func TestNewFunctionCatalog_EmptyAggregateGroupMatchesNoAggregateHeader(t *testing.T) {
	cat := NewFunctionCatalog(
		[]string{}, // aggregate: empty
		[]string{"HoltWintersPrediction"},
		[]string{"Length", "Round"},
	)

	if cat.aggregateRe.MatchString("AnythingAtAll(x)") {
		t.Errorf("aggregateRe with an empty aggregate group must not match any header, matched %q", "AnythingAtAll(x)")
	}
	if !cat.functionRe.MatchString("Length(x)") {
		t.Error("functionRe should still match the real scalar name")
	}
}

func TestNewFunctionCatalog(t *testing.T) {
	cat := NewFunctionCatalog(
		[]string{"Avg", "Sum"},            // aggregate
		[]string{"HoltWintersPrediction"}, // analytic
		[]string{"Length", "Round"},       // scalar
	)

	// aggregateRe matches aggregate headers and captures name + inner args; not the others.
	if !cat.aggregateRe.MatchString("Avg(x)") {
		t.Error("aggregateRe should match Avg(x)")
	}
	if cat.aggregateRe.MatchString("Length(x)") {
		t.Error("aggregateRe should not match a scalar function")
	}

	// functionRe matches non-aggregate (analytic/scalar) headers, and excludes aggregates.
	if !cat.functionRe.MatchString("Length(x)") || !cat.functionRe.MatchString("HoltWintersPrediction(x)") {
		t.Error("functionRe should match analytic/scalar functions")
	}
	if cat.functionRe.MatchString("Avg(x)") {
		t.Error("functionRe should NOT match an aggregate (matched separately, kept disjoint)")
	}

	// names covers all three groups (used for field-vs-function disambiguation).
	for _, n := range []string{"Avg", "Sum", "HoltWintersPrediction", "Length", "Round"} {
		if !cat.names[n] {
			t.Errorf("names should contain %q", n)
		}
	}
}

func TestDefaultFunctionCatalog(t *testing.T) {
	cat := DefaultFunctionCatalog()
	if cat == nil {
		t.Fatal("DefaultFunctionCatalog must not be nil")
	}
	// Sanity: known canonical names from each group are present.
	for _, n := range []string{"Avg", "Length", "HoltWintersPrediction"} {
		if !cat.names[n] {
			t.Errorf("default catalog missing %q", n)
		}
	}
	if !cat.aggregateRe.MatchString("Sum(ElapsedTime)") {
		t.Error("default aggregateRe should match Sum(ElapsedTime)")
	}
}
