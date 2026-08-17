# meshIQ Platform data source for Grafana

Query the **meshIQ Platform** from Grafana. This backend data source runs
[jKQL](https://www.meshiq.com/) queries against the meshIQ dataservice and returns the results
as native Grafana data frames — events, logs, metrics, and messaging-middleware data (Kafka,
IBM MQ, RabbitMQ, Solace, TIBCO EMS, ActiveMQ, and more).

## Features

- **jKQL query editor** with syntax highlighting and optional autocomplete.
- **Table and time series** result formats. Time-bucketed results pivot into series for graph
  panels.
- **Template variables** — drive dashboard variables from a jKQL query, or list item types and
  fields. Multi-value variables expand to a quoted list, ready for `In ($variable)`.
- **Alerting and annotations** — alert rules evaluate on the backend; annotation queries can
  overlay meshIQ events on any dashboard panel.
- **Repository selection** per data source and per query, for multi-repository accounts.
- The dashboard **time range is applied automatically** to every query.

## Requirements

- Grafana 12.3 or newer.
- A meshIQ dataservice endpoint and an access token.

## Configuration

Add a **meshIQ Platform** data source and fill in:

| Field | Meaning |
|---|---|
| **Service URL** | Base URL of the meshIQ dataservice, e.g. `https://your-host:8084/ds-api`. |
| **Access Token** | API token for the dataservice. Sent as the `X-API-Key` header; stored encrypted. |
| **Default repository** | Repository used by queries that don't pick their own. Loaded after a valid connection. |
| **Enable completion** | Turn on jKQL autocomplete, served by a separate completion service. |
| **Completion service URL** | Base URL of the jKQL autocomplete service, e.g. `http://your-host:7580`. |

Click **Save & Test**. On success the page also shows the server's **version** and its
**maximum result rows** limit.

## Writing queries

Type a jKQL statement and press **Ctrl/Cmd+Enter** (or click away) to run it:

```sql
-- Recent log entries
Get Log

-- Errors only
Get Log Where Severity In ('ERROR', 'FATAL')

-- Slowest events
Get Top 10 Event Fields ResourceName, ElapsedTime Sort By ElapsedTime Desc

-- Log volume over time (choose "Format as: Time series")
Get Number Of Log Group By ReportTime Bucketed By 1 hour

-- Filter on a custom property
Get Log Where Properties('Region') In ('us-east', 'us-west')
```

The dashboard time range is added to every query automatically, so you normally don't write
time filters.

### Template variables

Create a variable with one of the query types:

- **jKQL query** — the first result column becomes the variable's values.
- **Item types** — all queryable item types (Log, Event, KAFKA_BROKER, …).
- **Fields** — the fields of one item type, including custom properties.

A multi-value variable interpolates as `'a', 'b', 'c'`, so use it like
`Where Severity In ($severity)`.

## Provisioning

```yaml
apiVersion: 1
datasources:
  - name: meshIQ Platform
    type: meshiq-platform-datasource
    jsonData:
      serviceUrl: https://your-host:8084/ds-api
      repositoryID: DefaultRepo$YourOrg
    secureJsonData:
      accessToken: $MESHIQ_ACCESS_TOKEN
```

## Getting help

- Source and issues: <https://github.com/Nastel/meshiq-platform-datasource>
- meshIQ: <https://www.meshiq.com/>

Apache License 2.0.
