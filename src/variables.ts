import {
  CustomVariableSupport,
  DataFrame,
  DataQueryRequest,
  DataQueryResponse,
  FieldType,
  MetricFindValue,
  toDataFrame,
} from '@grafana/data';
import { from, map, Observable } from 'rxjs';

import type { DataSource } from './datasource';
import { VariableQueryEditor } from './components/VariableQueryEditor';
import { MeshIqVariableQuery } from './types';

/**
 * Custom template-variable support. Renders {@link VariableQueryEditor} and resolves a variable
 * query by delegating to the datasource's `metricFindQuery`, exposing the results as a single-field
 * data frame whose values Grafana turns into the variable's options.
 *
 * Replaces the deprecated `DataSourcePlugin.setVariableQueryEditor`.
 */
export class MeshIqVariableSupport extends CustomVariableSupport<DataSource> {
  constructor(private readonly datasource: DataSource) {
    super();
  }

  editor = VariableQueryEditor;

  query(request: DataQueryRequest<MeshIqVariableQuery>): Observable<DataQueryResponse> {
    const target = request.targets[0];
    return from(this.datasource.metricFindQuery(target, { range: request.range })).pipe(
      map((values) => ({ data: [toValuesFrame(values)] }))
    );
  }
}

/** Wraps metric-find results in a single string field named `text`. */
function toValuesFrame(values: MetricFindValue[]): DataFrame {
  return toDataFrame({
    fields: [{ name: 'text', type: FieldType.string, values: values.map((v) => v.text) }],
  });
}
