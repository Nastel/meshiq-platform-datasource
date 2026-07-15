import React, { ChangeEvent, KeyboardEvent } from 'react';
import { InlineField, TextArea } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from '../datasource';
import { MeshIqDataSourceOptions, MeshIqQuery } from '../types';

type Props = QueryEditorProps<DataSource, MeshIqQuery, MeshIqDataSourceOptions>;

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const onJkqlChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    onChange({ ...query, jkql: event.target.value });
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
      event.preventDefault();
      onRunQuery();
    }
  };

  return (
    <InlineField
      label="jKQL"
      labelWidth={10}
      grow
      tooltip="Enter a jKQL query. Press Ctrl/Cmd+Enter or click away to run."
    >
      <TextArea
        id="query-editor-jkql"
        value={query.jkql || ''}
        onChange={onJkqlChange}
        onBlur={onRunQuery}
        onKeyDown={onKeyDown}
        rows={4}
        placeholder="Get Events"
      />
    </InlineField>
  );
}
