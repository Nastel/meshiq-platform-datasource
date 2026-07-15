import React, { useEffect, useRef, useState } from 'react';
import { CodeEditor, Combobox, ComboboxOption, InlineField, Monaco, MonacoEditor, RadioButtonGroup } from '@grafana/ui';
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
  // Tracked separately from repositoriesOptions.length: an empty list means either "still
  // loading", "genuinely no repositories", or "the fetch failed" — without this flag a failed
  // fetch would hide the Repository field entirely instead of surfacing the error, taking the
  // required-field validation down with it.
  const [repositoriesError, setRepositoriesError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    datasource
      .listRepositories()
      .then((repos) => {
        if (!cancelled) {
          setRepositoriesOptions(buildRepositoriesComboboxOptions(repos));
          setRepositoriesError(false);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setRepositoriesOptions([]);
          setRepositoriesError(true);
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

  // Keep the latest props in refs for every callback CodeEditor binds once at mount — the Monaco
  // command, the completion resolver, and onChange (registered via model.onDidChangeContent, never
  // re-bound). Those callbacks must observe current values through the refs: a direct closure
  // would spread the mount-time query and revert fields changed since mount.
  const onRunQueryRef = useRef(onRunQuery);
  const repositoryRef = useRef(repositoryValue);
  const datasourceRef = useRef(datasource);
  const queryRef = useRef(query);
  const onChangeRef = useRef(onChange);
  useEffect(() => {
    onRunQueryRef.current = onRunQuery;
    repositoryRef.current = repositoryValue;
    datasourceRef.current = datasource;
    queryRef.current = query;
    onChangeRef.current = onChange;
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
    onChangeRef.current({ ...queryRef.current, jkql: value });
  };

  const onJkqlBlur = (value: string) => {
    onChange({ ...query, jkql: value });
    onRunQuery();
  };

  const onEditorDidMount = (editor: MonacoEditor, monaco: Monaco) => {
    // editor.addCommand registers in Monaco's shared standalone keybinding service, not scoped to
    // this editor instance — with more than one jKQL editor mounted (Explore split view, several
    // query rows), the last-registered instance's handler would run for all of them. onKeyDown is
    // instance-scoped; this mirrors how CodeEditor itself binds its own Ctrl/Cmd+S handler.
    editor.onKeyDown((e) => {
      if (e.keyCode === monaco.KeyCode.Enter && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        onRunQueryRef.current();
      }
    });
    activateCompletion();
  };

  const onRepositoryChange = (selected: ComboboxOption) => {
    onChange({ ...query, repositoryID: selected.value });
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
      {(repositoriesOptions.length > 0 || repositoriesError) && (
        <InlineField
          label="Repository"
          labelWidth={18}
          required
          invalid={repositoriesError || !repositoryValue}
          error={repositoriesError ? 'Could not load repositories' : 'Select a repository'}
          tooltip="Repository to query. Defaults to the datasource's default repository."
        >
          <Combobox
            id="query-editor-repository"
            options={repositoriesOptions}
            value={repositoryValue}
            onChange={onRepositoryChange}
            invalid={repositoriesError || !repositoryValue}
            placeholder="Choose repository"
            width={40}
          />
        </InlineField>
      )}
    </>
  );
}
