import React, { ChangeEvent, useEffect, useRef, useState } from 'react';
import {
  CodeEditor,
  Combobox,
  ComboboxOption,
  InlineField,
  InlineSwitch,
  Monaco,
  MonacoEditor,
  RadioButtonGroup,
} from '@grafana/ui';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { DataSource } from '../datasource';
import { MeshIqDataSourceOptions, MeshIqFormat, MeshIqQuery } from '../types';
import { buildRepositoriesComboboxOptions } from '../utils';
import {
  clearJkqlCompletionHandler,
  JKQL_LANGUAGE_ID,
  registerJkqlLanguage,
  setJkqlCompletionHandler,
  SuggestionResolver,
} from '../completion';

type Props = QueryEditorProps<DataSource, MeshIqQuery, MeshIqDataSourceOptions>;

const FORMAT_OPTIONS: Array<SelectableValue<MeshIqFormat>> = [
  { label: 'Table', value: 'table' },
  { label: 'Time series', value: 'timeseries' },
];

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

  // Keep the latest run callback, repository and datasource in refs so the Monaco command and
  // completion resolver (bound at mount/focus) always observe current values without re-registering.
  const onRunQueryRef = useRef(onRunQuery);
  const repositoryRef = useRef(repositoryValue);
  const datasourceRef = useRef(datasource);
  useEffect(() => {
    onRunQueryRef.current = onRunQuery;
    repositoryRef.current = repositoryValue;
    datasourceRef.current = datasource;
  });

  // One stable resolver per editor (fresh values via the refs above; useState's lazy initializer
  // guarantees the identity never changes). The stable identity lets the unmount cleanup release
  // the shared handler only when this editor still owns it — deleting one query row must not kill
  // completion in a sibling row.
  const [resolver] = useState<SuggestionResolver>(
    () => (text: string, caret: number) => datasourceRef.current.getSuggestions(text, caret, repositoryRef.current)
  );
  useEffect(() => () => clearJkqlCompletionHandler(resolver), [resolver]);

  const activateCompletion = () => {
    setJkqlCompletionHandler(resolver);
  };

  const onJkqlChange = (value: string) => {
    onChange({ ...query, jkql: value });
  };

  const onJkqlBlur = (value: string) => {
    onChange({ ...query, jkql: value });
    onRunQuery();
  };

  const onEditorDidMount = (editor: MonacoEditor, monaco: Monaco) => {
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => onRunQueryRef.current());
    activateCompletion();
  };

  const onRepositoryChange = (selected: ComboboxOption) => {
    onChange({ ...query, repositoryID: selected.value });
    onRunQuery();
  };

  const onTraceChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, trace: event.currentTarget.checked });
    onRunQuery();
  };

  const onFormatChange = (format: MeshIqFormat) => {
    onChange({ ...query, format });
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
            language={JKQL_LANGUAGE_ID}
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
            onBeforeEditorMount={registerJkqlLanguage}
            onEditorDidMount={onEditorDidMount}
            onFocus={activateCompletion}
            onChange={onJkqlChange}
            onBlur={onJkqlBlur}
          />
        </div>
      </InlineField>
      <InlineField
        label="Format as"
        labelWidth={18}
        tooltip="Table returns rows as-is. Time series pivots time-bucketed results into series for graph panels."
      >
        <RadioButtonGroup options={FORMAT_OPTIONS} value={query.format ?? 'table'} onChange={onFormatChange} />
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
