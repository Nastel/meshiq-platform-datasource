import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

/** Fallback row cap used before the server's MaxResultRows is known. */
export const MAX_ROWS_LIMIT = 1000;

/** Repository name pre-selected as the default when present (identifier form "DefaultRepo$<org>"). */
export const DEFAULT_REPOSITORY_NAME = 'DefaultRepo';

export type MeshIqFormat = 'table' | 'timeseries';

export interface MeshIqQuery extends DataQuery {
  jkql?: string;
  locale?: string;
  timezone?: string;
  format?: MeshIqFormat;
  repositoryID?: string;
  maxRows?: number;
  /** Ask the dataservice to include query trace info (jk_trace). Undefined inherits the datasource default. */
  trace?: boolean;
}

export const DEFAULT_QUERY: Partial<MeshIqQuery> = {
  format: 'table',
};

/** Fallback item type for a "fields" template-variable query that doesn't name a table. */
export const DEFAULT_ITEM_TYPE = 'Log';

/**
 * How a template-variable query resolves its values.
 * - `tables`: list item types via /tables (jKQL `get items`).
 * - `fields`: list a table's fields via /fields (jKQL `get fields for <table>`).
 * - `query`: run a raw jKQL query; the first column becomes the values.
 */
export type MeshIqVariableQueryType = 'tables' | 'fields' | 'query';

export interface MeshIqVariableQuery extends DataQuery {
  type: MeshIqVariableQueryType;
  table?: string; // used by the `fields` type
  jkql?: string; // used by the `query` type
}

export const DEFAULT_VARIABLE_QUERY: Partial<MeshIqVariableQuery> = {
  type: 'query',
  jkql: '',
};

/** One item type ("table") returned by /tables. */
export interface MeshIqTable {
  name: string;
}

/** One field returned by /fields. `custom` marks Properties-derived fields. */
export interface MeshIqField {
  name: string;
  type: string;
  custom: boolean;
}

/**
 * Options configured for each meshIQ Platform data source instance.
 */
export interface MeshIqDataSourceOptions extends DataSourceJsonData {
  serviceUrl?: string;
  /** Default repository applied to queries that don't select one. */
  repositoryID?: string;
  /** Default trace flag applied to queries that don't set their own. */
  trace?: boolean;
  /** Turns on jKQL autocomplete, proxied to completionServiceUrl. */
  enableCompletion?: boolean;
  /** Base URL of the jKQL autocomplete service. */
  completionServiceUrl?: string;
}

/**
 * One completion returned by the jKQL autocomplete service, proxied through the backend
 * `/suggestions` resource. `kind` is the service's enum name (StatementType, ItemType, Limit,
 * Keyword, Field, Function, Operator, Token, Separator, Totals). `deleteBackwards` is the number
 * of characters before the caret to replace when inserting.
 */
export interface MeshIqCompletionItem {
  label: string;
  insertText?: string;
  kind?: string;
  deleteBackwards?: number;
}

/**
 * Secret values, stored in the backend and never sent back to the frontend.
 */
export interface MeshIqSecureJsonData {
  accessToken?: string;
}
