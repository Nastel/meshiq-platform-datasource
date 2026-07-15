import { CoreApp, DataQueryRequest, DataQueryResponse, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, HealthCheckResult, HealthStatus, getTemplateSrv } from '@grafana/runtime';
import { Observable } from 'rxjs';

import { DEFAULT_QUERY, MeshIqDataSourceOptions, MeshIqQuery, MAX_ROWS_LIMIT } from './types';

export class DataSource extends DataSourceWithBackend<MeshIqQuery, MeshIqDataSourceOptions> {
  private readonly defaultRepositoryID: string;
  // Cached from the health check ("Get Params" -> MaxResultRows/ServiceVersion) and the
  // repositories resource.
  private maxRowsLimit = MAX_ROWS_LIMIT;
  private serviceVersion = '';
  private repositories: string[] = [];

  constructor(instanceSettings: DataSourceInstanceSettings<MeshIqDataSourceOptions>) {
    super(instanceSettings);
    this.defaultRepositoryID = instanceSettings.jsonData.repositoryID || '';
  }

  getDefaultQuery(_: CoreApp): Partial<MeshIqQuery> {
    return DEFAULT_QUERY;
  }

  query(request: DataQueryRequest<MeshIqQuery>): Observable<DataQueryResponse> {
    // Cap rows at min(server limit, panel maxDataPoints) — a panel never renders more points
    // than maxDataPoints, so asking for more just moves bytes.
    const maxRows =
      request.maxDataPoints === undefined ? this.maxRowsLimit : Math.min(this.maxRowsLimit, request.maxDataPoints);

    const targets = request.targets.map((target) => ({
      ...target,
      maxRows,
      repositoryID: target.repositoryID || this.defaultRepositoryID || undefined,
    }));

    return super.query({ ...request, targets });
  }

  // ---- Health & repositories -----------------------------------------------

  // A health check always tests the SAVED configuration; callers must not invoke it per
  // keystroke, or it would test the previous URL and mislead.
  async callHealthCheck(): Promise<HealthCheckResult> {
    const response = await super.callHealthCheck();

    const maxResultRows = response?.details?.['MaxResultRows'];
    this.maxRowsLimit = maxResultRows ? parseInt(String(maxResultRows), 10) || MAX_ROWS_LIMIT : MAX_ROWS_LIMIT;

    // Shown as-is, whatever the server reports.
    const version = response?.details?.['ServiceVersion'] ?? response?.details?.['ApiVersion'];
    this.serviceVersion = version ? String(version) : '';

    if (response.status === HealthStatus.OK) {
      await this.loadRepositories();
    } else {
      this.repositories = [];
    }
    return response;
  }

  private async loadRepositories(): Promise<string[]> {
    try {
      this.repositories = await this.getResource<string[]>('repositories');
    } catch {
      this.repositories = [];
    }
    return this.repositories;
  }

  getRepositories(): string[] {
    return this.repositories;
  }

  /** Returns the cached repositories, loading them on demand (used by the query editor). */
  async listRepositories(): Promise<string[]> {
    if (this.repositories.length) {
      return this.repositories;
    }
    return this.loadRepositories();
  }

  getMaxRowsLimit(): number {
    return this.maxRowsLimit;
  }

  /** Dataservice version + build from the last health check ("" until one succeeds). */
  getServiceVersion(): string {
    return this.serviceVersion;
  }

  getDefaultRepositoryID(): string {
    return this.defaultRepositoryID;
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
