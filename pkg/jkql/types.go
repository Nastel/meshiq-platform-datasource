package jkql

import "fmt"

// CompCodeType is a jKQL completion code. The dataservice reports one both as
// the response completion code in the "jk_ccode" field (SUCCESS -> HTTP 200, ERROR -> HTTP 400) and
// as the in-body result "status" (alongside an optional "status-msg"). It is a plain domain enum,
// usable anywhere.
type CompCodeType = string

const (
	SUCCESS CompCodeType = "SUCCESS"
	WARNING CompCodeType = "WARNING"
	ERROR   CompCodeType = "ERROR"
)

// ParseIssues collects wire-shape violations found while converting a result set. It threads
// through the converter functions (via DataModel.Issues below) so any of them can record one;
// List renders the collected issues in first-seen order, with a count suffix for repeats, for the
// caller to log.
type ParseIssues struct {
	counts map[string]int
	order  []string
}

// Add records one occurrence of an issue. Safe on a nil receiver, so hand-built models
// (tests, fixtures) need no collector.
func (p *ParseIssues) Add(issue string) {
	if p == nil {
		return
	}
	if p.counts == nil {
		p.counts = make(map[string]int)
	}
	if p.counts[issue] == 0 {
		p.order = append(p.order, issue)
	}
	p.counts[issue]++
}

// List renders the collected issues in first-seen order, with a count suffix for repeats.
// Returns nil (also on a nil receiver) when nothing was recorded.
func (p *ParseIssues) List() []string {
	if p == nil || len(p.order) == 0 {
		return nil
	}
	list := make([]string, 0, len(p.order))
	for _, issue := range p.order {
		if n := p.counts[issue]; n > 1 {
			issue = fmt.Sprintf("%s (x%d)", issue, n)
		}
		list = append(list, issue)
	}
	return list
}

// DataModel is the column-oriented representation of a dataservice result set, ready to be
// turned into a Grafana data frame.
type DataModel struct {
	RowCount      int          `json:"rowCount"`
	TotalRowCount int          `json:"totalRowCount"`
	Status        CompCodeType `json:"status"`    // in-body SUCCESS/WARNING/ERROR
	StatusMsg     string       `json:"statusMsg"` // e.g. "Only returning N of M rows"
	// Headers, and the map keys below, are raw column headers — not just field names. A header can
	// be any jKQL expression (a function call, a cast, a custom field), so it stays a plain string.
	Headers   []string                 `json:"headers"`
	Label     map[string]string        `json:"label"`     // header -> display label
	DataTypes map[string]DataType      `json:"dataTypes"` // header -> jKQL data type
	Rows      []map[string]interface{} `json:"rows"`
	// Issues is a pointer: the model is copied by value into the frame builder, and every
	// copy must feed the same collector.
	Issues *ParseIssues `json:"-"`
}
