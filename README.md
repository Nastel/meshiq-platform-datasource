# meshIQ Platform data source for Grafana

A backend data source plugin that lets you query the **meshIQ platform** dataservice from
Grafana — events, logs, metrics, and messaging-middleware data. Query results are returned as
native Grafana data frames, so they can be visualized in panels, used in Explore, drive template
variables, and back alert rules.

## Requirements

- Grafana >= 12.3.0
- Access to a meshIQ dataservice endpoint and an access token.

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for the dev loop (devcontainer, `docker-compose up`,
watch/debug scripts).

## License

Apache License 2.0 — see [LICENSE](LICENSE).
