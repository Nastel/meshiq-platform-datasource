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

  it('the Table combobox stops showing a loading spinner once a failed fetch settles, even though the result is an empty list', async () => {
    const onChange = jest.fn();
    const datasource = {
      fetchTables: jest.fn().mockRejectedValue(new Error('boom')),
    } as unknown as DataSource;
    const query: MeshIqVariableQuery = { refId: 'A', type: 'fields', jkql: '' };

    render(<VariableQueryEditor query={query} onChange={onChange} datasource={datasource} />);

    await waitFor(() => expect(datasource.fetchTables).toHaveBeenCalled());
    // tables.length === 0 both while loading and after this failed fetch settles — only a real
    // loading flag (not tables.length) can tell the two apart, so the spinner must go away.
    await waitFor(() => expect(screen.queryByTestId('icon-spinner')).not.toBeInTheDocument());
  });
});
