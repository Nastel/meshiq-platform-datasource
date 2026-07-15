# meshIQ Platform data source for Grafana

A backend data source plugin that lets you query the **meshIQ platform** dataservice from
Grafana — events, logs, metrics, and messaging-middleware data. Query results are returned as
native Grafana data frames, so they can be visualized in panels, used in Explore, drive template
variables, and back alert rules.

## Requirements

- Grafana >= 12.3.0
- Access to a meshIQ dataservice endpoint and an access token.

## Configuration

Add a **meshIQ Platform** data source and provide:

- **Service URL** — the base URL of the meshIQ dataservice.
- **Access Token** — an API token for the dataservice (stored in `secureJsonData`).

Click **Save & Test** to verify the connection.

Full user documentation — query examples, template variables, provisioning — is in
[src/README.md](src/README.md) (the page shown in the Grafana plugin catalog).

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for the dev loop (devcontainer, `docker-compose up`,
watch/debug scripts) and the test suites.

## Releasing

Releases are built and signed by the `release` GitHub workflow when a version tag (`v*`) is
pushed. Signing needs a Grafana Cloud access policy token with the `plugins:write` scope,
stored as the **`GRAFANA_ACCESS_POLICY_TOKEN`** repository secret. Without it the workflow
still builds, but produces an unsigned bundle. See the
[Grafana signing docs](https://grafana.com/developers/plugin-tools/publish-a-plugin/sign-a-plugin).

## License

Apache License 2.0 — see [LICENSE](LICENSE).
