package jkql

import "testing"

func TestParseIssues_AddAndList(t *testing.T) {
	var issues ParseIssues

	if got := issues.List(); got != nil {
		t.Errorf("List on an empty collector = %v, want nil", got)
	}

	issues.Add("bad column")
	issues.Add("bad column")
	issues.Add("another issue")

	got := issues.List()
	want := []string{"bad column (x2)", "another issue"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

func TestParseIssues_NilReceiverIsSafe(t *testing.T) {
	var issues *ParseIssues
	issues.Add("should not panic")
	if got := issues.List(); got != nil {
		t.Errorf("List on a nil collector = %v, want nil", got)
	}
}
