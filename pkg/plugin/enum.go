package plugin

import (
	"context"
	"net/http"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// enumValues returns the complete, ordinal-indexed value set of a built-in enum field (index = jKQL
// ordinal, value = name), memoized on the datasource. A cached empty slice means the field couldn't
// be resolved; callers treat an empty result as "no full set" and fall back to the compact table.
func (d *Datasource) enumValues(ctx context.Context, field string, options MeshIqDataSourceOptions) []string {
	d.enumCacheMu.RLock()
	cached, ok := d.enumCache[field]
	d.enumCacheMu.RUnlock()
	if ok {
		return cached
	}

	text := fetchEnumValues(ctx, d.httpClient, field, options)
	if text == nil {
		text = []string{} // cache the negative result so we don't re-query a non-enumerable field
	}

	d.enumCacheMu.Lock()
	d.enumCache[field] = text
	d.enumCacheMu.Unlock()
	return text
}

// fetchEnumValues runs "GET ENUMERATION FOR <field>" and builds an ordinal-indexed name table from
// the (ID, Name) result rows. Returns nil if the query fails or the field isn't an enum type.
func fetchEnumValues(ctx context.Context, httpClient *http.Client, field string, options MeshIqDataSourceOptions) []string {
	queryModel := BuildEnumValuesQueryModel(field)
	queryModel.RepositoryID = options.RepositoryID
	result, err := queryDataService(ctx, httpClient, queryModel, options)
	if err != nil {
		// The negative result is cached, so this degradation (enum ordinals shown without their
		// full name table) lasts for the instance lifetime — worth a warning, once per field.
		log.DefaultLogger.FromContext(ctx).Warn("could not load enum values; falling back to the ordinals seen in the result",
			"field", field, "repo", queryModel.RepositoryID, "error", err)
		return nil
	}

	model := jkql.BuildDataModel(result, nil)
	logParseIssues(ctx, queryModel, model)

	maxOrdinal := -1
	type entry struct {
		ordinal int
		name    string
	}
	entries := make([]entry, 0, model.RowCount)
	for _, row := range model.Rows {
		ordinal, ok := grafanaInt(model, row, jkql.ID)
		if !ok {
			continue
		}
		name, ok := grafanaString(model, row, jkql.NAME)
		if !ok {
			continue
		}
		entries = append(entries, entry{ordinal, name})
		if ordinal > maxOrdinal {
			maxOrdinal = ordinal
		}
	}
	if maxOrdinal < 0 {
		log.DefaultLogger.FromContext(ctx).Debug("enum query returned no usable rows", "field", field, "repo", queryModel.RepositoryID)
		return nil
	}

	// "GET ENUMERATION FOR <field>" returns the field's complete value set with contiguous
	// ordinals, so the Text table is inherently dense (no gaps). Only the data rows are ever
	// sparse, and they just index into this full table by ordinal.
	text := make([]string, maxOrdinal+1)
	for _, e := range entries {
		if e.ordinal >= 0 && e.ordinal < len(text) {
			text[e.ordinal] = e.name
		}
	}
	return text
}
