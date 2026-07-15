import React, { ChangeEvent, useEffect, useState } from 'react';
import { Combobox, ComboboxOption, InlineField, InlineSwitch, Input, SecretInput } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { getDataSourceSrv, HealthStatus } from '@grafana/runtime';

import { DataSource } from '../datasource';
import { MeshIqDataSourceOptions, MeshIqSecureJsonData, MAX_ROWS_LIMIT, DEFAULT_REPOSITORY_NAME } from '../types';
import { buildRepositoriesComboboxOptions } from '../utils';

interface Props extends DataSourcePluginOptionsEditorProps<MeshIqDataSourceOptions, MeshIqSecureJsonData> {}

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;

  const [datasource, setDatasource] = useState<DataSource>();
  const [connectionValid, setConnectionValid] = useState(false);
  const [repositoriesOptions, setRepositoriesOptions] = useState<ComboboxOption[]>([]);
  const [maxRowsLimit, setMaxRowsLimit] = useState<number>(MAX_ROWS_LIMIT);
  const [serviceVersion, setServiceVersion] = useState('');

  // Resolve the saved datasource instance so we can run health checks / load repositories.
  // Re-resolve after every save (options.version bumps) — the resolved instance carries the
  // settings it was created with, so a stale one would health-check the previous configuration.
  useEffect(() => {
    getDataSourceSrv()
      .get(options.uid)
      .then((ds) => setDatasource(ds as DataSource))
      .catch(() => setDatasource(undefined));
  }, [options.uid, options.version]);

  const configured = !!jsonData.serviceUrl && !!secureJsonFields?.accessToken;

  // Run a health check when the SAVED configuration changes (options.version bumps on save; the
  // instance is re-resolved above). The backend can only test saved settings — running this per
  // keystroke would test the previous URL and mislead, so this deliberately runs on Save & Test
  // only, not on every change to the fields above.
  useEffect(() => {
    if (!datasource || !configured) {
      return;
    }
    let cancelled = false;
    datasource
      .callHealthCheck()
      .then((response) => {
        if (cancelled) {
          return;
        }
        const ok = response.status === HealthStatus.OK;
        setConnectionValid(ok);
        setRepositoriesOptions(ok ? buildRepositoriesComboboxOptions(datasource.getRepositories()) : []);
        setMaxRowsLimit(datasource.getMaxRowsLimit());
        setServiceVersion(datasource.getServiceVersion());
      })
      .catch(() => {
        if (!cancelled) {
          setConnectionValid(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [datasource, configured, options.version]);

  // On the connection becoming valid, pre-select the conventional default repository
  // ("DefaultRepo$<org>") if present and none is chosen yet, so the required field is satisfied.
  useEffect(() => {
    if (!connectionValid || !datasource || jsonData.repositoryID) {
      return;
    }
    const preferred = datasource.getRepositories().find((repo) => repo.split('$')[0] === DEFAULT_REPOSITORY_NAME);
    if (preferred) {
      onOptionsChange({ ...options, jsonData: { ...jsonData, repositoryID: preferred } });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connectionValid]);

  const onServiceUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, jsonData: { ...jsonData, serviceUrl: event.target.value } });
  };

  const onRepositoryChange = (selected: ComboboxOption) => {
    onOptionsChange({ ...options, jsonData: { ...jsonData, repositoryID: selected.value } });
  };

  const onTraceChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, jsonData: { ...jsonData, trace: event.currentTarget.checked } });
  };

  // Secure field (only sent to the backend).
  const onAccessTokenChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, secureJsonData: { ...options.secureJsonData, accessToken: event.target.value } });
  };

  const onResetAccessToken = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: { ...options.secureJsonFields, accessToken: false },
      secureJsonData: { ...options.secureJsonData, accessToken: '' },
    });
  };

  return (
    <>
      <InlineField
        label="Service URL"
        labelWidth={28}
        required
        interactive
        invalid={!jsonData.serviceUrl}
        error="Enter the service URL"
        tooltip="Base URL of the meshIQ dataservice"
      >
        <Input
          id="config-editor-service-url"
          onChange={onServiceUrlChange}
          value={jsonData.serviceUrl ?? ''}
          placeholder="https://your-meshiq-host"
          width={40}
          autoComplete="off"
        />
      </InlineField>
      <InlineField
        label="Access Token"
        labelWidth={28}
        required
        interactive
        invalid={!secureJsonFields?.accessToken && !secureJsonData?.accessToken}
        error="Enter the access token"
        tooltip="API token for the dataservice"
      >
        <SecretInput
          required
          id="config-editor-access-token"
          isConfigured={!!secureJsonFields?.accessToken}
          value={secureJsonData?.accessToken}
          placeholder="Enter your access token"
          width={40}
          onReset={onResetAccessToken}
          onChange={onAccessTokenChange}
          autoComplete="off"
        />
      </InlineField>

      {configured && connectionValid && (
        <>
          {repositoriesOptions.length > 0 && (
            <InlineField
              label="Default repository"
              labelWidth={28}
              required
              interactive
              invalid={!jsonData.repositoryID}
              error="Select a default repository"
              tooltip="Repository applied to queries that don't select their own"
            >
              <Combobox
                id="config-editor-repository"
                options={repositoriesOptions}
                value={jsonData.repositoryID}
                onChange={onRepositoryChange}
                invalid={!jsonData.repositoryID}
                placeholder="Choose default repository"
                width={40}
              />
            </InlineField>
          )}
          {serviceVersion && (
            <InlineField
              label="Service version"
              labelWidth={28}
              disabled
              tooltip="Dataservice version and build, reported by the server (ServiceVersion / ApiBuildTime)"
            >
              <Input id="config-editor-service-version" value={serviceVersion} width={40} readOnly />
            </InlineField>
          )}
          <InlineField
            label="Max rows limit"
            labelWidth={28}
            disabled
            tooltip="Server-side maximum result rows (MaxResultRows)."
          >
            <Input id="config-editor-max-rows" value={maxRowsLimit} width={40} readOnly />
          </InlineField>
        </>
      )}

      {configured && (
        <InlineField
          label="Trace"
          labelWidth={28}
          interactive
          tooltip="Default: ask the dataservice to include query trace info (jk_trace). Overridable per query."
        >
          <InlineSwitch id="config-editor-trace" value={jsonData.trace ?? false} onChange={onTraceChange} />
        </InlineField>
      )}
    </>
  );
}
