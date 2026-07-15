import { DataQueryRequest, ScopedVars } from '@grafana/data';
import { setTemplateSrv, TemplateSrv } from '@grafana/runtime';
import { of } from 'rxjs';

import { DataSource } from './datasource';
import { MeshIqQuery } from './types';

// jsdom doesn't implement crypto.randomUUID; runJkqlForValues uses it to build a request id.
beforeAll(() => {
  if (!globalThis.crypto?.randomUUID) {
    Object.defineProperty(globalThis, 'crypto', {
      value: { ...globalThis.crypto, randomUUID: () => '00000000-0000-0000-0000-000000000000' },
    });
  }
});

// A minimal TemplateSrv good enough to exercise real interpolation (including the custom
// `formatJkqlVariable` passed to `.replace()`), without pulling in Grafana's full templating
// engine or a live dashboard context.
function makeTemplateSrv(scopedVars: ScopedVars = {}): TemplateSrv {
  return {
    getVariables: () => [],
    containsTemplate: (target?: string) => !!target && /\$\w+/.test(target),
    updateTimeRange: () => {},
    replace: (target?: string, vars?: ScopedVars, format?: string | Function) => {
      if (!target) {
        return '';
      }
      const merged = { ...scopedVars, ...vars };
      return target.replace(/\$(\w+)/g, (match, name) => {
        const entry = merged[name];
        if (!entry) {
          return match;
        }
        if (typeof format === 'function') {
          return format(entry.value);
        }
        return String(entry.value);
      });
    },
  };
}

function makeDataSource(): DataSource {
  return new DataSource({
    id: 1,
    uid: 'meshiq-1',
    type: 'meshiq-platform-datasource',
    name: 'meshIQ Platform',
    meta: {} as never,
    jsonData: {},
    readOnly: false,
    access: 'proxy',
  } as never);
}

describe('DataSource.applyTemplateVariables (jKQL quoting via formatJkqlVariable)', () => {
  beforeEach(() => {
    setTemplateSrv(makeTemplateSrv());
  });

  it('leaves a jkql with no variables untouched', () => {
    const ds = makeDataSource();
    const query: MeshIqQuery = { refId: 'A', jkql: 'Get Events' };

    expect(ds.applyTemplateVariables(query, {}).jkql).toBe('Get Events');
  });

  it('interpolates a single-value variable raw, without adding quotes', () => {
    const ds = makeDataSource();
    const query: MeshIqQuery = { refId: 'A', jkql: 'Get Events WHERE Severity = $sev' };
    const scopedVars: ScopedVars = { sev: { text: 'ERROR', value: 'ERROR' } };

    expect(ds.applyTemplateVariables(query, scopedVars).jkql).toBe('Get Events WHERE Severity = ERROR');
  });

  it('interpolates a multi-value variable as a quoted, comma-separated list', () => {
    const ds = makeDataSource();
    const query: MeshIqQuery = { refId: 'A', jkql: 'Get Events WHERE Severity IN ($sev)' };
    const scopedVars: ScopedVars = { sev: { text: 'ERROR + WARNING', value: ['ERROR', 'WARNING'] } };

    expect(ds.applyTemplateVariables(query, scopedVars).jkql).toBe("Get Events WHERE Severity IN ('ERROR', 'WARNING')");
  });

  it('backslash-escapes both backslashes and single quotes inside a quoted value', () => {
    const ds = makeDataSource();
    const query: MeshIqQuery = { refId: 'A', jkql: 'Get Events WHERE Message = $msg' };
    const scopedVars: ScopedVars = { msg: { text: "it's a \\test", value: ["it's a \\test"] } };

    expect(ds.applyTemplateVariables(query, scopedVars).jkql).toBe("Get Events WHERE Message = 'it\\'s a \\\\test'");
  });
});

describe('DataSource.metricFindQuery scopedVars threading', () => {
  it('passes the caller-supplied scopedVars through to the underlying query request', async () => {
    const ds = makeDataSource();
    setTemplateSrv(makeTemplateSrv());

    const querySpy = jest.spyOn(ds, 'query').mockReturnValue(of({ data: [] }));

    const scopedVars: ScopedVars = { env: { text: 'prod', value: 'prod' } };
    await ds.metricFindQuery('Get Events FIELDS Severity', { scopedVars });

    expect(querySpy).toHaveBeenCalledTimes(1);
    const request = querySpy.mock.calls[0][0] as DataQueryRequest<MeshIqQuery>;
    expect(request.scopedVars).toBe(scopedVars);
  });

  it('defaults scopedVars to an empty object when the caller supplies none', async () => {
    const ds = makeDataSource();
    setTemplateSrv(makeTemplateSrv());

    const querySpy = jest.spyOn(ds, 'query').mockReturnValue(of({ data: [] }));

    await ds.metricFindQuery('Get Events FIELDS Severity');

    const request = querySpy.mock.calls[0][0] as DataQueryRequest<MeshIqQuery>;
    expect(request.scopedVars).toEqual({});
  });

  it('treats a query that interpolates to empty (given the caller scopedVars) as empty, without querying', async () => {
    const ds = makeDataSource();
    setTemplateSrv(makeTemplateSrv());

    const querySpy = jest.spyOn(ds, 'query').mockReturnValue(of({ data: [] }));

    const result = await ds.metricFindQuery('$empty', { scopedVars: { empty: { text: '', value: '' } } });

    expect(result).toEqual([]);
    expect(querySpy).not.toHaveBeenCalled();
  });

  it('flattens list() array values and dedupes into one option per distinct value', async () => {
    const ds = makeDataSource();
    setTemplateSrv(makeTemplateSrv());

    jest.spyOn(ds, 'query').mockReturnValue(
      of({
        data: [
          {
            fields: [{ values: [['a', 'b', 'a']] }],
          },
        ],
      }) as never
    );

    const result = await ds.metricFindQuery({ refId: 'A', type: 'query', jkql: 'Get Events' });

    expect(result).toEqual([{ text: 'a' }, { text: 'b' }]);
  });
});
