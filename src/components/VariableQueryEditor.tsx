import React, { ChangeEvent, useEffect, useState } from 'react';
import { Combobox, ComboboxOption, InlineField, TextArea } from '@grafana/ui';

import type { DataSource } from '../datasource';
import { MeshIqVariableQuery, MeshIqVariableQueryType, DEFAULT_VARIABLE_QUERY } from '../types';

interface Props {
  query: MeshIqVariableQuery;
  onChange: (query: MeshIqVariableQuery) => void;
  datasource: DataSource;
}

const TYPE_OPTIONS: Array<ComboboxOption<MeshIqVariableQueryType>> = [
  { label: 'Tables', value: 'tables', description: 'List item types (jKQL: get items)' },
  { label: 'Fields', value: 'fields', description: 'List a table’s fields (jKQL: get fields for <table>)' },
  { label: 'jKQL query', value: 'query', description: 'Run a jKQL query; the first column becomes the values' },
];

export function VariableQueryEditor({ query, onChange, datasource }: Props) {
  const model: MeshIqVariableQuery = { ...DEFAULT_VARIABLE_QUERY, ...query };
  const [tables, setTables] = useState<ComboboxOption[]>([]);

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
        }
      })
      .catch(() => {
        if (!cancelled) {
          setTables([]);
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
        <InlineField label="Table" labelWidth={20} tooltip="Item type whose fields become the values">
          <Combobox
            id="meshiq-variable-table"
            options={tables}
            value={model.table}
            onChange={onTableChange}
            placeholder="Select an item type"
            loading={tables.length === 0}
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
