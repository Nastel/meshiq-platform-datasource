package plugin

import (
	"context"
	"errors"
	"net/http"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// functionCatalog returns this datasource's jKQL function catalog, memoized on the datasource. It
// loads the server's function set via `Get Functions`; on any failure the caller gets the built-in
// default so queries still work. The converter never loads it itself — only the datasource has the
// client. Only a real dataservice answer is cached — see cacheableFunctionCatalogError.
func (d *Datasource) functionCatalog(ctx context.Context, options MeshIqDataSourceOptions) *jkql.FunctionCatalog {
	d.functionCatalogCacheMu.RLock()
	cached := d.functionCatalogCache
	d.functionCatalogCacheMu.RUnlock()
	if cached != nil {
		return cached
	}

	cat, err := fetchFunctionCatalog(ctx, d.httpClient, options)
	if cat == nil {
		if !cacheableFunctionCatalogError(ctx, err) {
			return jkql.DefaultFunctionCatalog() // transient failure; let the next call retry
		}
		cat = jkql.DefaultFunctionCatalog() // genuine negative answer: cache it, use the fallback
	}

	d.functionCatalogCacheMu.Lock()
	d.functionCatalogCache = cat
	d.functionCatalogCacheMu.Unlock()
	return cat
}

// cacheableFunctionCatalogError reports whether a `Get Functions` fetch's failure is a genuine
// negative answer worth caching for the instance lifetime (the dataservice's own error envelope —
// a queryError, or no error at all — the endpoint answered, just with nothing usable), as opposed
// to a fetch that never really got an answer: ctx was canceled/timed out, or the call failed at
// the transport level (connection refused, DNS failure, a brief outage). Only the former should be
// memoized; caching the latter would pin a transient condition (e.g. the dataservice briefly
// unreachable at first dashboard load) as a permanent negative for the rest of the instance's
// lifetime.
func cacheableFunctionCatalogError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if err == nil {
		return true
	}
	var qe *queryError
	return errors.As(err, &qe)
}

// fetchFunctionCatalog runs `Get Functions` and builds a catalog from the Name/Type columns,
// grouping names by the server's function categories. A name reported under more than one type
// (e.g. Avg is both Aggregate and Analytic) is placed once, preferring Aggregate, so the aggregate
// and non-aggregate sets stay disjoint. Returns a nil catalog (caller falls back) on any error or
// empty name set; the error tells the caller whether that negative is cacheable (see
// cacheableFunctionCatalogError). The response has no function-call headers, so it parses with
// the default catalog (nil) — no chicken-and-egg.
func fetchFunctionCatalog(ctx context.Context, httpClient *http.Client, options MeshIqDataSourceOptions) (*jkql.FunctionCatalog, error) {
	queryModel := BuildFunctionsQueryModel()
	queryModel.RepositoryID = options.RepositoryID
	result, err := queryDataService(ctx, httpClient, queryModel, options)
	if err != nil {
		log.DefaultLogger.FromContext(ctx).Warn("could not load jKQL functions; using built-in fallback",
			"repo", queryModel.RepositoryID, "error", err)
		return nil, err
	}

	model := jkql.BuildDataModel(result, nil)
	logParseIssues(ctx, queryModel, model)

	typesByName := make(map[string]map[string]bool)
	for _, row := range model.Rows {
		name, ok := grafanaString(model, row, jkql.NAME)
		if !ok || name == "" {
			continue
		}
		typ, _ := grafanaString(model, row, "Type")
		if typesByName[name] == nil {
			typesByName[name] = make(map[string]bool)
		}
		typesByName[name][typ] = true
	}

	var aggregate, analytic, scalar []string
	for name, types := range typesByName {
		switch {
		case types["Aggregate"]:
			aggregate = append(aggregate, name)
		case types["Analytic"]:
			analytic = append(analytic, name)
		default: // Scalar, and anything the server may add later
			scalar = append(scalar, name)
		}
	}

	if len(aggregate)+len(analytic)+len(scalar) == 0 {
		log.DefaultLogger.FromContext(ctx).Warn("get functions returned no usable names; using built-in fallback",
			"repo", queryModel.RepositoryID)
		return nil, nil // a real negative answer (the dataservice responded, just with nothing usable): cacheable
	}
	return jkql.NewFunctionCatalog(aggregate, analytic, scalar), nil
}
