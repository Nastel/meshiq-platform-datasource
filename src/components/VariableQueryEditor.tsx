import React, { ChangeEvent, useEffect, useState } from 'react';
import { Combobox, ComboboxOption, InlineField, TextArea } from '@grafana/ui';

import type { DataSource } from '../datasource';
import { MeshIqVariableQuery, MeshIqVariableQueryType, DEFAULT_VARIABLE_QUERY } from '../types';

interface Props {
  // Grafana's CustomVariableSupport types this as MeshIqVariableQuery, but Grafana core
  // initializes every brand-new query variable's query to '' and passes that straight through
  // unless the datasource implements getDefaultQuery() (this one doesn't) — so a bare string
  // arrives here on the very first use of a Query variable, not just from an old saved query.
  // metricFindQuery in datasource.ts handles the same shape for the same reason.
  query: MeshIqVariableQuery | string;
  onChange: (query: MeshIqVariableQuery) => void;
  datasource: DataSource;
}

const TYPE_OPTIONS: Array<ComboboxOption<MeshIqVariableQueryType>> = [
  { label: 'Tables', value: 'tables', description: 'List item types (jKQL: get items)' },
  { label: 'Fields', value: 'fields', description: 'List a table’s fields (jKQL: get fields for <table>)' },
  { label: 'jKQL query', value: 'query', description: 'Run a jKQL query; the first column becomes the values' },
];

export function VariableQueryEditor({ query, onChange, datasource }: Props) {
  // See the Props.query comment: a brand-new query variable arrives here as a bare string.
  // Spreading a string yields indexed-character keys instead of a jkql field — the editor would
  // show blank and silently drop the text on any edit. refId 'A' applies only when the incoming
  // query had none (DataQuery requires one).
  const normalizedQuery: Partial<MeshIqVariableQuery> = typeof query === 'string' ? { jkql: query } : query;
  const model: MeshIqVariableQuery = { refId: 'A', type: 'query', ...DEFAULT_VARIABLE_QUERY, ...normalizedQuery };
  const [tables, setTables] = useState<ComboboxOption[]>([]);
  // tables.length can't distinguish "still fetching" from a settled empty/failed fetch; without
  // this flag the combobox would show a loading spinner forever in the settled cases.
  const [tablesLoading, setTablesLoading] = useState(false);
  // Tracked separately from tables.length: an empty list means either "still loading",
  // "genuinely no fields", or "the fetch failed" — without this flag a failed fetch would look
  // identical to a real empty result, giving no signal that something went wrong.
  const [tablesError, setTablesError] = useState(false);
  // The type a fetch is in flight for (or has last settled for), so a render can tell "type just
  // changed to fields, a fetch needs to start" apart from "already loading/loaded for this type" —
  // set during render (React's documented pattern for adjusting state as a prop changes), not
  // inside the effect, so the effect's own setState calls stay confined to its async callbacks.
  const [fetchType, setFetchType] = useState<MeshIqVariableQueryType>();
  if (model.type === 'fields' && fetchType !== 'fields') {
    setFetchType('fields');
    setTablesLoading(true);
  } else if (model.type !== 'fields' && fetchType !== undefined) {
    setFetchType(undefined);
  }

  // Load the item type list when the editor switches to (or opens on) the Fields type.
  useEffect(() => {
    if (model.type !== 'fields') {
      return;
    }
    let cancelled = false;
    datasource
      .fetchTables()
      .then((rows) => {
        if (!cancelled) {
          setTables(rows.map((t) => ({ label: t.name, value: t.name })));
          setTablesError(false);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setTables([]);
          setTablesError(true);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setTablesLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [model.type, datasource]);

  const onTypeChange = (selected: ComboboxOption<MeshIqVariableQueryType>) => {
    onChange({ ...model, type: selected.value });
  };

  const onTableChange = (selected: ComboboxOption) => {
    onChange({ ...model, table: selected.value });
  };

  const onJkqlChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    onChange({ ...model, jkql: event.target.value });
  };

  return (
    <>
      <InlineField label="Query type" labelWidth={20} tooltip="How this variable resolves its values">
        <Combobox
          id="meshiq-variable-type"
          options={TYPE_OPTIONS}
          value={model.type}
          onChange={onTypeChange}
          width={40}
        />
      </InlineField>

      {model.type === 'fields' && (
        <InlineField
          label="Table"
          labelWidth={20}
          tooltip="Item type whose fields become the values"
          invalid={tablesError}
          error={tablesError ? 'Could not load item types' : undefined}
        >
          <Combobox
            id="meshiq-variable-table"
            options={tables}
            value={model.table}
            onChange={onTableChange}
            placeholder="Select an item type"
            loading={tablesLoading}
            invalid={tablesError}
            createCustomValue
            width={40}
          />
        </InlineField>
      )}

      {model.type === 'query' && (
        <InlineField label="jKQL" labelWidth={20} grow tooltip="First column of the result becomes the variable values">
          <TextArea
            id="meshiq-variable-jkql"
            value={model.jkql || ''}
            onChange={onJkqlChange}
            rows={3}
            placeholder="Get Events FIELDS Severity"
          />
        </InlineField>
      )}
    </>
  );
}
