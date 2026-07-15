import React from 'react';
import { act, render, screen, waitFor } from '@testing-library/react';
import type { Monaco, MonacoEditor } from '@grafana/ui';

import { QueryEditor } from './QueryEditor';
import type { DataSource } from '../datasource';
import type { MeshIqQuery } from '../types';

// The Repository Combobox measures text width via canvas, which jsdom doesn't implement.
beforeAll(() => {
  (HTMLCanvasElement.prototype as unknown as { getContext: () => unknown }).getContext = () => ({
    measureText: (text: string) => ({ width: text.length * 7 }),
  });
});

// Grafana's CodeEditor registers onChange once at editor mount (model.onDidChangeContent) and
// never re-binds it, so the editor keeps calling the mount-render closure forever. The mock
// reproduces exactly that: it keeps the first onChange it ever sees and ignores later renders.
let mockMountOnChange: ((value: string) => void) | undefined;
let mockOnEditorDidMount: ((editor: MonacoEditor, monaco: Monaco) => void) | undefined;

jest.mock('@grafana/ui', () => {
  const actual = jest.requireActual('@grafana/ui');
  return {
    ...actual,
    CodeEditor: (props: {
      onChange?: (value: string) => void;
      onEditorDidMount?: (editor: MonacoEditor, monaco: Monaco) => void;
    }) => {
      if (!mockMountOnChange) {
        mockMountOnChange = props.onChange;
      }
      if (!mockOnEditorDidMount) {
        mockOnEditorDidMount = props.onEditorDidMount;
      }
      return null;
    },
  };
});

// A fake Monaco editor instance: onKeyDown is instance-scoped (captures the callback so the test
// can fire a synthetic keydown), unlike addCommand, which would register in Monaco's shared
// standalone keybinding service — the exact global-leak this fix replaces.
function makeFakeMonacoEditor() {
  let keyDownHandler: ((e: { keyCode: number; ctrlKey: boolean; metaKey: boolean; preventDefault: () => void }) => void) | undefined;
  const addCommand = jest.fn();
  return {
    editor: {
      onKeyDown: (handler: typeof keyDownHandler) => {
        keyDownHandler = handler;
      },
      addCommand,
    } as unknown as MonacoEditor,
    fireCtrlEnter: () => {
      keyDownHandler?.({ keyCode: 3 /* monaco.KeyCode.Enter */, ctrlKey: true, metaKey: false, preventDefault: jest.fn() });
    },
    addCommand,
  };
}

const fakeMonaco = { KeyCode: { Enter: 3 }, KeyMod: { CtrlCmd: 0 } } as unknown as Monaco;

function makeDatasource(): DataSource {
  return {
    listRepositories: jest.fn().mockResolvedValue([]),
    getDefaultRepositoryID: jest.fn().mockReturnValue('repo-1'),
  } as unknown as DataSource;
}

describe('QueryEditor', () => {
  beforeEach(() => {
    mockMountOnChange = undefined;
    mockOnEditorDidMount = undefined;
  });

  it('typing commits the current query fields, not the ones from editor-mount time', async () => {
    const onChange = jest.fn();
    const onRunQuery = jest.fn();
    const datasource = makeDatasource();
    const query: MeshIqQuery = { refId: 'A', jkql: 'get events', repositoryID: 'repo-1' };

    const { rerender } = render(
      <QueryEditor query={query} onChange={onChange} onRunQuery={onRunQuery} datasource={datasource} />
    );
    await waitFor(() => expect(datasource.listRepositories).toHaveBeenCalled());
    expect(mockMountOnChange).toBeDefined();

    // The user picks a different repository; Grafana re-renders with the updated query.
    rerender(
      <QueryEditor
        query={{ ...query, repositoryID: 'repo-2' }}
        onChange={onChange}
        onRunQuery={onRunQuery}
        datasource={datasource}
      />
    );

    // Then types in the editor: the mount-time onChange must still see repositoryID=repo-2.
    act(() => mockMountOnChange!('get events fields Severity'));

    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ repositoryID: 'repo-2', jkql: 'get events fields Severity' })
    );
  });

  it('binds Ctrl/Cmd+Enter via the editor-scoped onKeyDown, not the shared addCommand registry', async () => {
    const onChange = jest.fn();
    const onRunQuery = jest.fn();
    const datasource = makeDatasource();
    const query: MeshIqQuery = { refId: 'A', jkql: 'get events', repositoryID: 'repo-1' };

    render(<QueryEditor query={query} onChange={onChange} onRunQuery={onRunQuery} datasource={datasource} />);
    await waitFor(() => expect(datasource.listRepositories).toHaveBeenCalled());
    expect(mockOnEditorDidMount).toBeDefined();

    const { editor, fireCtrlEnter, addCommand } = makeFakeMonacoEditor();
    act(() => mockOnEditorDidMount!(editor, fakeMonaco));

    expect(addCommand).not.toHaveBeenCalled();

    fireCtrlEnter();
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('shows the Repository field in an error state instead of hiding it when the repositories fetch fails', async () => {
    const onChange = jest.fn();
    const onRunQuery = jest.fn();
    const datasource = {
      listRepositories: jest.fn().mockRejectedValue(new Error('boom')),
      getDefaultRepositoryID: jest.fn().mockReturnValue('repo-1'),
    } as unknown as DataSource;
    const query: MeshIqQuery = { refId: 'A', jkql: 'get events', repositoryID: 'repo-1' };

    render(<QueryEditor query={query} onChange={onChange} onRunQuery={onRunQuery} datasource={datasource} />);

    await waitFor(() => expect(datasource.listRepositories).toHaveBeenCalled());
    expect(await screen.findByText('Could not load repositories')).toBeInTheDocument();
  });
});
