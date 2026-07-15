import { CoreApp, DataQueryRequest, DataQueryResponse, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, HealthCheckResult, HealthStatus, getTemplateSrv } from '@grafana/runtime';
import { Observable } from 'rxjs';

import { DEFAULT_QUERY, MeshIqDataSourceOptions, MeshIqQuery, MeshIqCompletionItem, MAX_ROWS_LIMIT } from './types';

export class DataSource extends DataSourceWithBackend<MeshIqQuery, MeshIqDataSourceOptions> {
  private readonly defaultRepositoryID: string;
  private readonly defaultTrace: boolean;
  // Cached from the health check ("Get Params" -> MaxResultRows/ServiceVersion) and the
  // repositories resource.
  private maxRowsLimit = MAX_ROWS_LIMIT;
  private serviceVersion = '';
  private repositories: string[] = [];

  constructor(instanceSettings: DataSourceInstanceSettings<MeshIqDataSourceOptions>) {
    super(instanceSettings);
    this.defaultRepositoryID = instanceSettings.jsonData.repositoryID || '';
    this.defaultTrace = instanceSettings.jsonData.trace ?? false;
  }

  getDefaultQuery(_: CoreApp): Partial<MeshIqQuery> {
    return DEFAULT_QUERY;
  }

  // query injects the effective row cap (min of the server's MaxResultRows and the panel's
  // maxDataPoints) and the default repository into each target before running the backend query.
  query(request: DataQueryRequest<MeshIqQuery>): Observable<DataQueryResponse> {
    const maxRows =
      request.maxDataPoints === undefined ? this.maxRowsLimit : Math.min(this.maxRowsLimit, request.maxDataPoints);

    const targets = request.targets.map((target) => ({
      ...target,
      maxRows,
      repositoryID: target.repositoryID || this.defaultRepositoryID || undefined,
      // A per-query toggle (true/false) wins; only an untouched query inherits the datasource default.
      trace: target.trace ?? this.defaultTrace,
    }));

    return super.query({ ...request, targets });
  }

  // ---- Health & repositories -----------------------------------------------

  // callHealthCheck is only ever invoked by Grafana's Save & Test button (or once by the config
  // editor after a successful save) — never on every keystroke — so a health check always tests
  // the currently saved configuration, not one still being typed.
  async callHealthCheck(): Promise<HealthCheckResult> {
    const response = await super.callHealthCheck();

    const maxResultRows = response?.details?.['MaxResultRows'];
    this.maxRowsLimit = maxResultRows ? parseInt(String(maxResultRows), 10) || MAX_ROWS_LIMIT : MAX_ROWS_LIMIT;

    // Service version + build, e.g. "12.1.1_65 (built 2026-07-03T01:21:53)". The version value
    // already carries the build number after the underscore; ApiBuildTime adds when it was built.
    const version = response?.details?.['ServiceVersion'] ?? response?.details?.['ApiVersion'];
    const buildTime = response?.details?.['ApiBuildTime'];
    this.serviceVersion = version ? `${version}${buildTime ? ` (built ${buildTime})` : ''}` : '';

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

  getDefaultTrace(): boolean {
    return this.defaultTrace;
  }

  // ---- jKQL autocomplete ---------------------------------------------------

  /**
   * Fetches jKQL completions for the text up to `caretIndex`, proxied through the backend
   * `/suggestions` resource to the configured autocomplete service. Falls back to no suggestions
   * (rather than surfacing an error in the editor) when completion is disabled or the service is
   * unreachable.
   */
  async getSuggestions(jkql: string, caretIndex: number, repositoryID?: string): Promise<MeshIqCompletionItem[]> {
    try {
      const params: Record<string, string | number> = { jk_query: jkql, jk_position: caretIndex };
      const repo = repositoryID || this.defaultRepositoryID;
      if (repo) {
        params.jk_repo = repo;
      }
      return await this.getResource<MeshIqCompletionItem[]>('suggestions', params);
    } catch {
      return [];
    }
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
