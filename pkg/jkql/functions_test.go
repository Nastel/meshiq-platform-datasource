package jkql

import "testing"

func TestOrNames(t *testing.T) {
	got := orNames([]string{"a", "b"}, []string{"c"})
	if got != "a|b|c" {
		t.Errorf("orNames = %q, want %q", got, "a|b|c")
	}
	if orNames() != "" {
		t.Errorf("orNames() of nothing should be empty")
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
