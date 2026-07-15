import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { VariableQueryEditor } from './VariableQueryEditor';
import type { DataSource } from '../datasource';
import type { MeshIqVariableQuery } from '../types';

// The "Query type" Combobox measures text width via canvas, which jsdom doesn't implement.
beforeAll(() => {
  (HTMLCanvasElement.prototype as unknown as { getContext: () => unknown }).getContext = () => ({
    measureText: (text: string) => ({ width: text.length * 7 }),
  });
});

function makeDatasource(): DataSource {
  return {
    fetchTables: jest.fn().mockResolvedValue([]),
  } as unknown as DataSource;
}

describe('VariableQueryEditor', () => {
  it('renders a bare-string query (e.g. a brand-new variable, which Grafana core initializes to "") as its jKQL text, not blank', () => {
    const onChange = jest.fn();
    const datasource = makeDatasource();

    render(<VariableQueryEditor query={'Get Events FIELDS Severity'} onChange={onChange} datasource={datasource} />);

    expect(screen.getByDisplayValue('Get Events FIELDS Severity')).toBeInTheDocument();
  });

  it('editing a bare-string query commits a structured query with the original text preserved', () => {
    const onChange = jest.fn();
    const datasource = makeDatasource();

    render(<VariableQueryEditor query={'Get Events FIELDS Severity'} onChange={onChange} datasource={datasource} />);

    fireEvent.change(screen.getByDisplayValue('Get Events FIELDS Severity'), {
      target: { value: 'Get Events FIELDS Severity WHERE Severity > 3' },
    });

    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ type: 'query', jkql: 'Get Events FIELDS Severity WHERE Severity > 3' })
    );
  });

  it('still renders a structured query unchanged (no regression from the bare-string handling)', () => {
    const onChange = jest.fn();
    const datasource = makeDatasource();
    const query: MeshIqVariableQuery = { refId: 'A', type: 'query', jkql: 'Get Events' };

    render(<VariableQueryEditor query={query} onChange={onChange} datasource={datasource} />);

    expect(screen.getByDisplayValue('Get Events')).toBeInTheDocument();
  });

  it('surfaces a distinct error when the table fetch fails, instead of looking like an empty result', async () => {
    // Regression coverage for the underlying loading-flag bug (tablesLoading stuck true forever)
    // lives in the "re-entering Fields" test below, which can observe the loading flag's effect on
    // a second fetch directly. This test pins the separate tablesError signal: without it, a failed
    // fetch settles to the exact same tables===[] state as a genuinely empty result, giving the user
    // no indication anything went wrong.
    const onChange = jest.fn();
    const fetchTables = jest.fn().mockRejectedValue(new Error('boom'));
    const datasource = { fetchTables } as unknown as DataSource;
    const query: MeshIqVariableQuery = { refId: 'A', type: 'fields', jkql: '' };

    render(<VariableQueryEditor query={query} onChange={onChange} datasource={datasource} />);

    await waitFor(() => expect(fetchTables).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByText('Could not load item types')).toBeInTheDocument());
  });

  it('clears the table-fetch error once a subsequent fetch succeeds', async () => {
    const onChange = jest.fn();
    const fetchTables = jest.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce([{ name: 'Log' }]);
    const datasource = { fetchTables } as unknown as DataSource;

    const fieldsQuery: MeshIqVariableQuery = { refId: 'A', type: 'fields', jkql: '' };
    const { rerender } = render(<VariableQueryEditor query={fieldsQuery} onChange={onChange} datasource={datasource} />);

    await waitFor(() => expect(screen.getByText('Could not load item types')).toBeInTheDocument());

    const tablesQuery: MeshIqVariableQuery = { refId: 'A', type: 'tables', jkql: '' };
    rerender(<VariableQueryEditor query={tablesQuery} onChange={onChange} datasource={datasource} />);
    rerender(<VariableQueryEditor query={fieldsQuery} onChange={onChange} datasource={datasource} />);

    await waitFor(() => expect(fetchTables).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByText('Could not load item types')).not.toBeInTheDocument());
  });

  it('fetches the table list again on re-entering Fields a second time, not just the first', async () => {
    // Without the fetchType re-entry guard, tablesLoading's lazy `useState(false)` would only
    // ever be set to true once — but more importantly, without it the effect's own dependency
    // array ([model.type, datasource]) is what actually drives re-fetching here, so this test's
    // real job is pinning that leaving Fields and coming back re-triggers the effect at all.
    const onChange = jest.fn();
    const fetchTables = jest
      .fn()
      .mockResolvedValueOnce([{ name: 'Log' }])
      .mockResolvedValueOnce([{ name: 'Log' }, { name: 'Event' }]);
    const datasource = { fetchTables } as unknown as DataSource;

    const fieldsQuery: MeshIqVariableQuery = { refId: 'A', type: 'fields', jkql: '' };
    const { rerender } = render(<VariableQueryEditor query={fieldsQuery} onChange={onChange} datasource={datasource} />);

    await waitFor(() => expect(fetchTables).toHaveBeenCalledTimes(1));

    const tablesQuery: MeshIqVariableQuery = { refId: 'A', type: 'tables', jkql: '' };
    rerender(<VariableQueryEditor query={tablesQuery} onChange={onChange} datasource={datasource} />);
    rerender(<VariableQueryEditor query={fieldsQuery} onChange={onChange} datasource={datasource} />);

    await waitFor(() => expect(fetchTables).toHaveBeenCalledTimes(2));
  });
});
