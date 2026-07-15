import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

/** Fallback row cap used before the server's MaxResultRows is known. */
export const MAX_ROWS_LIMIT = 1000;

/** Repository name pre-selected as the default when present (identifier form "DefaultRepo$<org>"). */
export const DEFAULT_REPOSITORY_NAME = 'DefaultRepo';

export interface MeshIqQuery extends DataQuery {
  jkql?: string;
  locale?: string;
  timezone?: string;
  repositoryID?: string;
  maxRows?: number;
}

export const DEFAULT_QUERY: Partial<MeshIqQuery> = {};

/**
 * Options configured for each meshIQ Platform data source instance.
 */
export interface MeshIqDataSourceOptions extends DataSourceJsonData {
  serviceUrl?: string;
  /** Default repository applied to queries that don't select one. */
  repositoryID?: string;
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
