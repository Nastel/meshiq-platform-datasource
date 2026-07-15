package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource is the meshIQ Platform datasource instance.
type Datasource struct {
	httpClient *http.Client
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

	return &Datasource{httpClient: cl}, nil
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

	result, err := queryDataService(ctx, d.httpClient, *queryModel, *options)
	if err != nil {
		// Both cases are downstream (the dataservice, not the plugin): an error envelope means
		// the service rejected the query (bad jKQL, bad token) — a bad-request; anything else
		// is a transport/HTTP failure — a bad gateway. Log with the query and date range so a
		// failure seen on a panel can be found in the server log and reproduced — the panel
		// error alone doesn't say what ran.
		logger := log.DefaultLogger.FromContext(ctx)
		status := backend.StatusBadGateway
		var qe *queryError
		if errors.As(err, &qe) {
			status = backend.StatusBadRequest
			// Usually a mistake in the query (or an expired token), not an outage — warn.
			logger.Warn("dataservice rejected the query",
				"query", queryModel.JKQL, "date", queryModel.Date, "error", err)
		} else {
			logger.Error("dataservice request failed",
				"query", queryModel.JKQL, "date", queryModel.Date, "error", err)
		}
		return backend.ErrDataResponseWithSource(status, backend.ErrorSourceDownstream, err.Error())
	}

	dataModel := jkql.BuildDataModel(result)
	frame := jkql.BuildDataFrame(dataModel)
	logParseIssues(ctx, *queryModel, dataModel)
	frame = jkql.FinalizeFrame(frame, queryModel.JKQL)
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

	dataModel := jkql.BuildDataModel(response)
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
