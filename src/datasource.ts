import { CoreApp, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';

import { DEFAULT_QUERY, MeshIqDataSourceOptions, MeshIqQuery } from './types';

export class DataSource extends DataSourceWithBackend<MeshIqQuery, MeshIqDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<MeshIqDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<MeshIqQuery> {
    return DEFAULT_QUERY;
  }

  applyTemplateVariables(query: MeshIqQuery, scopedVars: ScopedVars): MeshIqQuery {
    return {
      ...query,
      jkql: query.jkql ? getTemplateSrv().replace(query.jkql, scopedVars) : query.jkql,
      locale: query.locale || getBrowserLocale(),
      timezone: query.timezone || getBrowserTimezone(),
    };
  }

  filterQuery(query: MeshIqQuery): boolean {
    // Don't run empty queries.
    return !!query.jkql;
  }
}

function getBrowserLocale(): string {
  return navigator.language || '';
}

function getBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || '';
  } catch {
    return '';
  }
}
