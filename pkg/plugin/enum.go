package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// enumValues returns the complete, ordinal-indexed value set of a built-in enum field (index = jKQL
// ordinal, value = name), memoized on the datasource. A cached empty slice means the field couldn't
// be resolved; callers fall back to the compact table. Only a real dataservice answer is cached —
// see cacheableFetchError.
func (d *Datasource) enumValues(ctx context.Context, field string, options MeshIqDataSourceOptions) []string {
	d.enumCacheMu.RLock()
	cached, ok := d.enumCache[field]
	d.enumCacheMu.RUnlock()
	if ok {
		return cached
	}

	text, err := fetchEnumValues(ctx, d.httpClient, field, options)
	if text == nil {
		if !cacheableFetchError(ctx, err) {
			return []string{} // transient failure; answer this call, but let the next one retry
		}
		text = []string{} // genuine negative answer: cache it, don't re-query a non-enumerable field
	}

	d.enumCacheMu.Lock()
	d.enumCache[field] = text
	d.enumCacheMu.Unlock()
	return text
}

// cacheableFetchError reports whether a metadata fetch's failure is a genuine negative answer
// worth caching for the instance lifetime (the dataservice's own error envelope — a queryError, or
// no error at all — the endpoint answered, just with nothing usable), as opposed to a fetch that
// never really got an answer: ctx was canceled/timed out, or the call failed at the transport
// level (connection refused, DNS failure, a brief outage). Only the former should be memoized;
// caching the latter would pin a transient condition (e.g. the dataservice briefly unreachable at
// first dashboard load) as a permanent negative for the rest of the instance's lifetime.
func cacheableFetchError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if err == nil {
		return true
	}
	var qe *queryError
	return errors.As(err, &qe)
}

// maxEnumOrdinal caps the ordinal a "Get Enumeration For" row is allowed to contribute. Real jKQL
// enums are small (well under a hundred values); this only guards against an ID column that isn't
// actually an enum ordinal (wrong field, misconfigured Service URL, bad response), which would
// otherwise size the ordinal-indexed table off a huge or garbage number and exhaust memory.
const maxEnumOrdinal = 4096

// fetchEnumValues runs "Get Enumeration For <field>" and builds an ordinal-indexed name table from
// the (ID, Name) result rows. Returns a nil slice if the query fails or the field isn't an enum
// type; the returned error tells the caller whether that negative is cacheable (see
// cacheableFetchError). Rows whose ordinal exceeds maxEnumOrdinal are skipped and recorded as a
// parse issue instead of growing the table.
func fetchEnumValues(ctx context.Context, httpClient *http.Client, field string, options MeshIqDataSourceOptions) ([]string, error) {
	queryModel := BuildEnumValuesQueryModel(field)
	queryModel.RepositoryID = options.RepositoryID
	result, err := queryDataService(ctx, httpClient, queryModel, options)
	if err != nil {
		// Warn either way: a cacheable negative degrades enum rendering for the instance
		// lifetime; a transient failure just retries on the next call.
		log.DefaultLogger.FromContext(ctx).Warn("could not load enum values; falling back to the ordinals seen in the result",
			"field", field, "repo", queryModel.RepositoryID, "error", err)
		return nil, err
	}

	model := jkql.BuildDataModel(result, nil)

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
		if ordinal < 0 || ordinal > maxEnumOrdinal {
			model.Issues.Add(fmt.Sprintf("field %s: enum ordinal %d exceeds the accepted range, skipped", field, ordinal))
			continue
		}
		entries = append(entries, entry{ordinal, name})
		if ordinal > maxOrdinal {
			maxOrdinal = ordinal
		}
	}
	logParseIssues(ctx, queryModel, model)
	if maxOrdinal < 0 {
		log.DefaultLogger.FromContext(ctx).Debug("enum query returned no usable rows", "field", field, "repo", queryModel.RepositoryID)
		return nil, nil // a real negative answer (the dataservice responded, just with nothing usable): cacheable
	}

	// Ordinal-indexed name table; an ordinal the response skipped keeps an empty slot, since
	// data rows index into the table by raw ordinal.
	text := make([]string, maxOrdinal+1)
	for _, e := range entries {
		if e.ordinal >= 0 && e.ordinal < len(text) {
			text[e.ordinal] = e.name
		}
	}
	return text, nil
}
