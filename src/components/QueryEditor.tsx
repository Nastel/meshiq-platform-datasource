import React, { useEffect, useRef, useState } from 'react';
import { CodeEditor, Combobox, ComboboxOption, InlineField, Monaco, MonacoEditor } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from '../datasource';
import { MeshIqDataSourceOptions, MeshIqQuery } from '../types';
import { buildRepositoriesComboboxOptions } from '../utils';

type Props = QueryEditorProps<DataSource, MeshIqQuery, MeshIqDataSourceOptions>;

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
  // command and onChange (registered via model.onDidChangeContent, never re-bound). Those
  // callbacks must observe current values through the refs: a direct closure would spread the
  // mount-time query and revert fields changed since mount.
  const onRunQueryRef = useRef(onRunQuery);
  const queryRef = useRef(query);
  const onChangeRef = useRef(onChange);
  useEffect(() => {
    onRunQueryRef.current = onRunQuery;
    queryRef.current = query;
    onChangeRef.current = onChange;
  });

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
  };

  const onRepositoryChange = (selected: ComboboxOption) => {
    onChange({ ...query, repositoryID: selected.value });
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
