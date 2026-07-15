import React, { ChangeEvent, useEffect, useRef, useState } from 'react';
import { CodeEditor, Combobox, ComboboxOption, InlineField, InlineSwitch, Monaco, MonacoEditor } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from '../datasource';
import { MeshIqDataSourceOptions, MeshIqQuery } from '../types';
import { buildRepositoriesComboboxOptions } from '../utils';

type Props = QueryEditorProps<DataSource, MeshIqQuery, MeshIqDataSourceOptions>;

export function QueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const [repositoriesOptions, setRepositoriesOptions] = useState<ComboboxOption[]>([]);

  useEffect(() => {
    let cancelled = false;
    datasource
      .listRepositories()
      .then((repos) => {
        if (!cancelled) {
          setRepositoriesOptions(buildRepositoriesComboboxOptions(repos));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setRepositoriesOptions([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [datasource]);

  // Pre-select the datasource's default repository on a new query so the (required) choice is
  // persisted on the query, not just displayed. Existing queries that already picked one are left
  // untouched. Runs once on mount.
  useEffect(() => {
    if (query.repositoryID) {
      return;
    }
    const defaultRepo = datasource.getDefaultRepositoryID();
    if (defaultRepo) {
      onChange({ ...query, repositoryID: defaultRepo });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const repositoryValue = query.repositoryID ?? datasource.getDefaultRepositoryID();

  // Keep the latest run callback in a ref so the Monaco command bound at mount always calls the
  // current onRunQuery without needing the editor to re-mount.
  const onRunQueryRef = useRef(onRunQuery);
  useEffect(() => {
    onRunQueryRef.current = onRunQuery;
  });

  const onJkqlChange = (value: string) => {
    onChange({ ...query, jkql: value });
  };

  const onJkqlBlur = (value: string) => {
    onChange({ ...query, jkql: value });
    onRunQuery();
  };

  const onEditorDidMount = (editor: MonacoEditor, monaco: Monaco) => {
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => onRunQueryRef.current());
  };

  const onRepositoryChange = (selected: ComboboxOption) => {
    onChange({ ...query, repositoryID: selected.value });
    onRunQuery();
  };

  const onTraceChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, trace: event.currentTarget.checked });
    onRunQuery();
  };

  return (
    <>
      <InlineField
        label="jKQL"
        labelWidth={18}
        grow
        tooltip="Enter a jKQL query. Press Ctrl/Cmd+Enter or click away to run."
      >
        <div style={{ width: '100%' }}>
          <CodeEditor
            language="plaintext"
            value={query.jkql || ''}
            height={90}
            showLineNumbers={false}
            showMiniMap={false}
            monacoOptions={{
              scrollBeyondLastLine: false,
              folding: false,
              lineNumbers: 'off',
              wordWrap: 'on',
              fontSize: 13,
            }}
            onEditorDidMount={onEditorDidMount}
            onChange={onJkqlChange}
            onBlur={onJkqlBlur}
          />
        </div>
      </InlineField>
      {repositoriesOptions.length > 0 && (
        <InlineField
          label="Repository"
          labelWidth={18}
          required
          invalid={!repositoryValue}
          error="Select a repository"
          tooltip="Repository to query. Defaults to the datasource's default repository."
        >
          <Combobox
            id="query-editor-repository"
            options={repositoriesOptions}
            value={repositoryValue}
            onChange={onRepositoryChange}
            invalid={!repositoryValue}
            placeholder="Choose repository"
            width={40}
          />
        </InlineField>
      )}
      <InlineField
        label="Trace"
        labelWidth={18}
        tooltip="Include query trace info (jk_trace). Defaults to the datasource setting."
      >
        <InlineSwitch
          id="query-editor-trace"
          value={query.trace ?? datasource.getDefaultTrace()}
          onChange={onTraceChange}
        />
      </InlineField>
    </>
  );
}
