import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export interface MeshIqQuery extends DataQuery {
  jkql?: string;
  locale?: string;
  timezone?: string;
  maxRows?: number;
}

export const DEFAULT_QUERY: Partial<MeshIqQuery> = {};

/**
 * Options configured for each meshIQ Platform data source instance.
 */
export interface MeshIqDataSourceOptions extends DataSourceJsonData {
  serviceUrl?: string;
}

/**
 * Secret values, stored in the backend and never sent back to the frontend.
 */
export interface MeshIqSecureJsonData {
  accessToken?: string;
}
