package plugin

import (
	"context"
	"net/http"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// functionCatalog returns this datasource's jKQL function catalog, memoized on the datasource. It
// loads the server's function set via `get functions` and injects it into jkql.BuildDataModel; on
// any failure it falls back to the built-in default catalog so queries still work. The converter
// never loads it itself — only the datasource has the client.
func (d *Datasource) functionCatalog(ctx context.Context, options MeshIqDataSourceOptions) *jkql.FunctionCatalog {
	d.functionCatalogCacheMu.RLock()
	cached := d.functionCatalogCache
	d.functionCatalogCacheMu.RUnlock()
	if cached != nil {
		return cached
	}

	cat := fetchFunctionCatalog(ctx, d.httpClient, options)
	if cat == nil {
		cat = jkql.DefaultFunctionCatalog() // negative result: don't re-query, use the fallback
	}

	d.functionCatalogCacheMu.Lock()
	d.functionCatalogCache = cat
	d.functionCatalogCacheMu.Unlock()
	return cat
}

// fetchFunctionCatalog runs `get functions` and builds a catalog from the Name/Type columns,
// grouping names by the server's function categories. A name may be reported under more than one
// type (e.g. Avg is both Aggregate and Analytic); it is placed once, preferring Aggregate, so the
// aggregate and non-aggregate sets stay disjoint. Returns nil (caller falls back) on any error or
// when the response yields no names. The `get functions` response has no function-call headers, so
// it parses with the default catalog (nil) — no chicken-and-egg.
func fetchFunctionCatalog(ctx context.Context, httpClient *http.Client, options MeshIqDataSourceOptions) *jkql.FunctionCatalog {
	queryModel := BuildFunctionsQueryModel()
	queryModel.RepositoryID = options.RepositoryID
	result, err := queryDataService(ctx, httpClient, queryModel, options)
	if err != nil {
		log.DefaultLogger.FromContext(ctx).Warn("could not load jKQL functions; using built-in fallback",
			"repo", queryModel.RepositoryID, "error", err)
		return nil
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
		return nil
	}
	return jkql.NewFunctionCatalog(aggregate, analytic, scalar)
}
