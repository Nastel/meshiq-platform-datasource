# meshIQ Platform data source for Grafana

Query the **meshIQ Platform** from Grafana. This backend data source runs jKQL queries and returns
the results as native Grafana data frames — application, infrastructure, and messaging-middleware
data collected by meshIQ across your environment.

meshIQ tracks messaging middleware — IBM MQ, IBM ACE/IIB, TIBCO EMS, Kafka, Solace, RabbitMQ,
ActiveMQ, Artemis, and others.

## Features

- **jKQL query editor** with syntax highlighting and optional autocomplete.
- **Table and time series** result formats. Time-bucketed results pivot into series for graph
  panels.
- **Template variables** — drive dashboard variables from a jKQL query, or list items and their
  fields. Multi-value variables expand to a quoted list, ready for `In ($variable)`.
- **Alerting and annotations** — alert rules evaluate on the backend; annotation queries can
  overlay events on any dashboard panel.

## Requirements

- Grafana 12.3 or newer.
- A meshIQ Platform endpoint and an access token.

## Install

Install from the Grafana plugin catalog (**Administration → Plugins and data → Plugins**, search
for "meshIQ Platform"), or manually with the Grafana CLI:

```bash
grafana-cli plugins install meshiq-platform-datasource
```

Then add a data source: **Connections → Add new connection → meshIQ Platform**.

## Configuration

Fill in the data source settings:

| Field | Required | Meaning |
|---|---|---|
| **Service URL** | Yes | Base URL of the meshIQ Platform, e.g. `https://your-meshIQ-host:8080/ds-api`. |
| **Access token** | Yes | API token for the meshIQ Platform; stored encrypted. |
| **Default repository** | Yes, once connected | Repository used by default; can be overridden per query. The picker loads after a valid connection. |
| **Enable completion** | No | Turn on jKQL autocomplete, served by a separate completion service. Off by default. |
| **Completion service URL** | Only if completion is enabled | Base URL of the jKQL autocomplete service, e.g. `http://your-meshIQ-host:7580`. |

Click **Save & Test**. On success the page also shows the server's version and its maximum
result-rows limit.

### Troubleshooting

> **"service URL is required" / "access token is required"**
> Shown only when the field is empty — fill it in and save again.

> **"meshIQ Platform returned a non-JSON response"**
> The Service URL didn't reach the jKQL endpoint — usually a wrong URL, a login/redirect page from
> a reverse proxy, or a proxy error page in front of the real service. Double check the Service URL
> (including any base path your deployment uses) and that the access token is valid.

> **"meshIQ Platform returned HTTP 4xx/5xx"**
> The request reached the service but was rejected — check the access token and that it's
> authorized for the repository you're querying.

> **"completion is enabled but no completion service URL is set"**
> Fill in the Completion service URL, or turn Enable completion off.

> **"could not reach the completion service" / "completion service returned HTTP ..."**
> The Completion service URL is unreachable or misconfigured. jKQL queries still run normally —
> only autocomplete is affected.

## Writing queries

Type a jKQL statement and press **Ctrl/Cmd+Enter** (or click away) to run it. A few examples,
adapted from the bundled showcase dashboards:

Kafka: number of consumer groups.

```text
Get Number Of KafkaConsumerGroup
```

Kafka: top 10 consumers by lag, excluding internal topics.

```text
Get KafkaConsumer Fields groupId, topicName, partitionId, lag
  Where topicName Not Starts With '__'
  Sort By lag Desc Range 1, 10
```

IBM MQ: local queues with a backlog, deepest first.

```text
Get IbmmqLocalQueue Fields Name As 'Queue', curQDepth As 'Current Depth', maxDepth As 'Max Depth'
  Where curQDepth > 0
  Sort By 'Current Depth' Desc
```

TIBCO EMS: server counts grouped by node type, for a pie chart.

```text
Get Number Of EmsServer Group By nodeType
```

collectd: average CPU utilization per hour, as a time series.

```text
Get COLLECTD.CPU Fields Avg(cpu.user.value) As 'Avg CPU'
  Where cpu.user.value Exists
  Group By MetricTime Bucketed By 1 Hour
```

The dashboard time range applies automatically, so you don't need time filters. Custom fields are
reached through `Properties('<name>')`; `Get Fields For <item>` lists them. `Get Items` lists every
queryable item type. For the full jKQL grammar, see your meshIQ Platform's own jKQL documentation.

### Format as

- **Table** — the default; every result renders as a table.
- **Time series** — pivots a time-bucketed result (`Group By <time field> Bucketed By ...`) into
  one series per group, for graph panels. Falls back to a table with a notice if the result isn't
  a recognizable time series.

### Template variables

Create a variable with one of the query types:

- **jKQL query** — the first result column becomes the variable's values, e.g.
  `Get KafkaCluster Fields Name Sort By Name`.
- **Items** — every queryable item type (KafkaCluster, IbmmqQueueManager, EmsServer, …).
- **Fields** — the fields of one item, including custom properties.

A multi-value variable interpolates as `'a', 'b', 'c'`, so use it like
`Where Name In ($cluster)`.

## Provisioning

```yaml
apiVersion: 1
datasources:
  - name: meshIQ Platform
    type: meshiq-platform-datasource
    jsonData:
      serviceUrl: https://your-meshIQ-host:8080/ds-api
      repositoryID: DefaultRepo$$YourOrg
      enableCompletion: true
      completionServiceUrl: http://your-meshIQ-host:7580
    secureJsonData:
      accessToken: $MESHIQ_ACCESS_TOKEN
```

A literal `$` in the repository ID (`RepositoryName$OrganizationName`) must be escaped as `$$`,
or Grafana's provisioning will try to expand it as an environment variable.

## Getting help

- Source and issues: <https://github.com/Nastel/meshiq-platform-datasource>
- meshIQ: <https://www.meshiq.com/>

Apache License 2.0.
