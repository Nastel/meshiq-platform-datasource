import React, { ChangeEvent, useEffect, useState } from 'react';
import { Combobox, ComboboxOption, InlineField, Input, SecretInput } from '@grafana/ui';
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
  // Which options.version the resolved instance above corresponds to. options.version bumps
  // immediately on save but re-resolving is async; the health-check effect waits for this to
  // catch up so it never runs against the stale pre-save instance.
  const [datasourceVersion, setDatasourceVersion] = useState<string | number | undefined>();
  const [connectionValid, setConnectionValid] = useState(false);
  const [repositoriesOptions, setRepositoriesOptions] = useState<ComboboxOption[]>([]);
  const [maxRowsLimit, setMaxRowsLimit] = useState<number>(MAX_ROWS_LIMIT);
  const [serviceVersion, setServiceVersion] = useState('');

  // Resolve the saved datasource instance so we can run health checks / load repositories.
  // Re-resolve after every save (options.version bumps) — the resolved instance carries the
  // settings it was created with, so a stale one would health-check the previous configuration.
  // getDataSourceSrv().get() shows as deprecated as of @grafana/runtime 13.1, in favor of
  // getDataSourceInstance/useDataSourceInstance — but those only exist under
  // @grafana/runtime/unstable, whose own doc comment says "THESE APIS MUST NOT BE USED IN
  // COMMUNITY PLUGINS." No sanctioned replacement exists yet; keep using .get() until one
  // ships in the stable API surface.
  useEffect(() => {
    const versionAtRequestTime = options.version;
    getDataSourceSrv()
      .get(options.uid)
      .then((ds) => {
        setDatasource(ds as DataSource);
        setDatasourceVersion(versionAtRequestTime);
      })
      .catch(() => {
        setDatasource(undefined);
        setDatasourceVersion(versionAtRequestTime);
      });
  }, [options.uid, options.version]);

  const configured = !!jsonData.serviceUrl && !!secureJsonFields?.accessToken;

  // Health-check the SAVED configuration only (the backend can only test saved settings — a
  // per-keystroke check would test the previous URL and mislead). Skips until the resolved
  // instance catches up to options.version, so the stale pre-save instance is never checked.
  useEffect(() => {
    if (!datasource || !configured || datasourceVersion !== options.version) {
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
  }, [datasource, configured, datasourceVersion, options.version]);

  // On a (re)connect that refreshes the repository list, pre-select the conventional default
  // repository ("DefaultRepo$<org>") if present and none is chosen yet, so the required field is
  // satisfied. Keyed on repositoriesOptions rather than connectionValid: connectionValid is a
  // boolean, so a later re-save that stays valid would be a no-op state update and this effect
  // would never re-run even though the repository list itself may have changed.
  useEffect(() => {
    if (!connectionValid || !datasource || jsonData.repositoryID) {
      return;
    }
    const preferred = datasource.getRepositories().find((repo) => repo.split('$')[0] === DEFAULT_REPOSITORY_NAME);
    if (preferred) {
      onOptionsChange({ ...options, jsonData: { ...jsonData, repositoryID: preferred } });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connectionValid, repositoriesOptions]);

  const onServiceUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, jsonData: { ...jsonData, serviceUrl: event.target.value } });
  };

  const onRepositoryChange = (selected: ComboboxOption) => {
    onOptionsChange({ ...options, jsonData: { ...jsonData, repositoryID: selected.value } });
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
          value={secureJsonData?.accessToken ?? ''}
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
              tooltip="Dataservice version, reported by the server (ServiceVersion)"
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
    </>
  );
}
