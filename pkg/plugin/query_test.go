package plugin

import (
	"strings"
	"testing"
)

// TestBuildItemsQueryModel_ExcludesAdminAndReferenceItems pins that the item-type picker query
// filters out admin item types (USER, ORGANIZATION, ACCESS_TOKEN, …), reference/catalog item
// types (Function, Keyword, Statement, …), and item types that don't support GET at all
// (GeoLocation, Server, Process, …) — confirmed live against the dataservice that
// Properties('isAdmin')/Properties('isReference')/StatementType are real, filterable fields on
// the "get items" result set.
func TestBuildItemsQueryModel_ExcludesAdminAndReferenceItems(t *testing.T) {
	jkql := BuildItemsQueryModel().JKQL
	if !strings.Contains(jkql, "Properties('isAdmin') = false") {
		t.Errorf("BuildItemsQueryModel().JKQL = %q, want isAdmin=false filter", jkql)
	}
	if !strings.Contains(jkql, "Properties('isReference') = false") {
		t.Errorf("BuildItemsQueryModel().JKQL = %q, want isReference=false filter", jkql)
	}
	if !strings.Contains(jkql, "StatementType = 'GET'") {
		t.Errorf("BuildItemsQueryModel().JKQL = %q, want StatementType = 'GET' filter", jkql)
	}
}
