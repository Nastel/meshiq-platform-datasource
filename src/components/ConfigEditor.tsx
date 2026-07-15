import React, { ChangeEvent } from 'react';
import { InlineField, Input, SecretInput } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { MeshIqDataSourceOptions, MeshIqSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<MeshIqDataSourceOptions, MeshIqSecureJsonData> {}

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;

  const onServiceUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({ ...options, jsonData: { ...jsonData, serviceUrl: event.target.value } });
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
        tooltip="Base URL of the meshIQ Platform"
      >
        <Input
          id="config-editor-service-url"
          onChange={onServiceUrlChange}
          value={jsonData.serviceUrl ?? ''}
          placeholder="https://your-meshIQ-host"
          width={40}
          autoComplete="off"
        />
      </InlineField>
      <InlineField
        label="Access token"
        labelWidth={28}
        required
        interactive
        invalid={!secureJsonFields?.accessToken && !secureJsonData?.accessToken}
        error="Enter the access token"
        tooltip="API token for the meshIQ Platform"
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
    </>
  );
}
