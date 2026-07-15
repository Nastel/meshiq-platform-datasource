import { DataSourcePlugin } from '@grafana/data';
import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import { MeshIqQuery, MeshIqDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, MeshIqQuery, MeshIqDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
