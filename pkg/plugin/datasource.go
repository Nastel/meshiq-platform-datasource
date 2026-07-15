package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/Nastel/meshiq-platform-datasource/pkg/jkql"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/experimental/concurrent"
)

// Params is a flat key/value map (used for the health-check result parameters).
type Params map[string]interface{}

// Make sure Datasource implements the required SDK interfaces.
var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource is the meshIQ Platform datasource instance.
type Datasource struct {
	httpClient      *http.Client
	resourceHandler backend.CallResourceHandler

	// functionCatalogCache is the jKQL function set loaded from the server ("get functions"),
	// memoized for the instance lifetime. Nil until first loaded; on load failure it is set to
	// the built-in default so queries still work. See functions.go.
	functionCatalogCacheMu sync.RWMutex
	functionCatalogCache   *jkql.FunctionCatalog

	// enumCache memoizes each built-in enum field's complete value set (ordinal-indexed names) from
	// "GET ENUMERATION FOR <field>". Enum definitions are static, so this is kept for the instance
	// lifetime. A cached empty slice marks a field whose values couldn't be resolved (don't retry).
	enumCacheMu sync.RWMutex
	enumCache   map[string][]string
}

// NewDatasource creates a new datasource instance with an HTTP client configured from
// Grafana's datasource settings (proxy, TLS, timeouts).
func NewDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	opts, err := settings.HTTPClientOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("http client options: %w", err)
	}

	cl, err := httpclient.New(opts)
	if err != nil {
		return nil, fmt.Errorf("httpclient new: %w", err)
	}

	d := &Datasource{httpClient: cl, enumCache: make(map[string][]string)}
	d.resourceHandler = d.newResourceHandler()
	return d, nil
}

// CallResource serves the frontend's non-query endpoints: /repositories and the /suggestions
// autocomplete proxy. See resources.go and completion.go.
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	return d.resourceHandler.CallResource(ctx, req, sender)
}

// Dispose cleans up datasource instance resources when the instance is replaced.
func (d *Datasource) Dispose() {
	if d.httpClient != nil {
		d.httpClient.CloseIdleConnections()
	}
}

// queryConcurrency bounds how many of one request's queries run against the dataservice at
// once. Grafana sends a panel's (or alert rule's) queries in one request; running them in
// parallel cuts the panel load time to the slowest query instead of the sum.
const queryConcurrency = 10

// QueryData handles multiple queries and returns multiple responses. Queries run concurrently
// via the SDK helper, which also recovers a panicking query into an error response for that
// query instead of crashing the plugin.
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return concurrent.QueryData(ctx, req, func(ctx context.Context, q concurrent.Query) backend.DataResponse {
		return d.query(ctx, q.PluginContext, q.DataQuery)
	}, queryConcurrency)
}

func (d *Datasource) query(ctx context.Context, pCtx backend.PluginContext, query backend.DataQuery) backend.DataResponse {
	response := backend.DataResponse{}

	queryModel, err := BuildQueryModel(query)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("query model: %v", err.Error()))
	}

	// Nothing to run.
	if queryModel.JKQL == "" {
		return response
	}

	options, err := BuildMeshIqDataSourceOptions(pCtx.DataSourceInstanceSettings)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("datasource options: %v", err.Error()))
	}

	// Fall back to the datasource defaults when the query doesn't carry its own repository or
	// trace flag.
	if queryModel.RepositoryID == "" {
		queryModel.RepositoryID = options.RepositoryID
	}
	if queryModel.Trace == nil {
		queryModel.Trace = &options.Trace
	}

	result, err := queryDataService(ctx, d.httpClient, *queryModel, *options)
	if err != nil {
		// Both cases are downstream (the dataservice, not the plugin): an error envelope means
		// the service rejected the query (bad jKQL, bad token) — a bad-request; anything else
		// is a transport/HTTP failure — a bad gateway. Log with the query, date range and
		// repository so a failure seen on a panel can be found in the server log and reproduced —
		// the panel error alone doesn't say what ran.
		logger := log.DefaultLogger.FromContext(ctx)
		status := backend.StatusBadGateway
		var qe *queryError
		if errors.As(err, &qe) {
			status = backend.StatusBadRequest
			// Usually a mistake in the query (or an expired token), not an outage — warn.
			logger.Warn("dataservice rejected the query",
				"query", queryModel.JKQL, "date", queryModel.Date, "repo", queryModel.RepositoryID, "error", err)
		} else {
			logger.Error("dataservice request failed",
				"query", queryModel.JKQL, "date", queryModel.Date, "repo", queryModel.RepositoryID, "error", err)
		}
		return backend.ErrDataResponseWithSource(status, backend.ErrorSourceDownstream, err.Error())
	}

	dataModel := jkql.BuildDataModel(result, d.functionCatalog(ctx, *options))

	frame := jkql.BuildDataFrame(dataModel, func(field string) []string {
		return d.enumValues(ctx, field, *options)
	})
	// Frame building can add issues too (variant envelopes), so log after it, once per query.
	logParseIssues(ctx, *queryModel, dataModel)
	frame = jkql.FinalizeFrame(frame, queryModel.JKQL, queryModel.Format)
	frame.RefID = query.RefID
	response.Frames = append(response.Frames, frame)

	return response
}

// CheckHealth verifies the datasource can reach the dataservice and authenticate, by
// running the "Get Params" jKQL statement.
func (d *Datasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	options, err := BuildMeshIqDataSourceOptions(req.PluginContext.DataSourceInstanceSettings)
	if err != nil {
		return newHealthError(err), nil
	}

	response, err := testDataService(ctx, d.httpClient, *options)
	if err != nil {
		return newHealthError(err), nil
	}

	if options.EnableCompletion && options.CompletionServiceUrl != "" {
		if err := testCompletionService(ctx, d.httpClient, *options); err != nil {
			return newHealthError(err), nil
		}
	}

	dataModel := jkql.BuildDataModel(response, nil)
	logParseIssues(ctx, BuildParamsQueryModel(), dataModel)
	details, err := json.Marshal(buildCheckHealthResult(dataModel))
	if err != nil {
		return newHealthError(err), nil
	}

	return &backend.CheckHealthResult{
		Status:      backend.HealthStatusOk,
		Message:     "Data source is working",
		JSONDetails: details,
	}, nil
}

// buildCheckHealthResult turns the "Get Params" result set into a flat key/value map
// (ParameterName -> ParameterValue) returned to the frontend as health details.
func buildCheckHealthResult(dataModel jkql.DataModel) Params {
	params := make(Params)
	if len(dataModel.Headers) < 2 {
		return params
	}

	keyHeader := dataModel.Headers[0]   // ParameterName
	valueHeader := dataModel.Headers[1] // ParameterValue
	for _, row := range dataModel.Rows {
		key, ok := jkql.ConvertToGrafanaValue(row[keyHeader], dataModel.DataTypes[keyHeader]).(string)
		if !ok {
			continue
		}
		params[key] = row[valueHeader]
	}

	return params
}

func newHealthError(err error) *backend.CheckHealthResult {
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusError,
		Message: err.Error(),
	}
}
