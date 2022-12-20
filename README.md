# xk6-output-clickhouse

`xk6-output-clickhouse` is a [k6 extension](https://k6.io/docs/extensions/) to send k6 metrics to clickhouse in a predefined schema.

# Install

You will need [go](https://golang.org/)

```bash

# Install xk6
go install go.k6.io/xk6/cmd/xk6@latest

# Build the k6 binary
xk6 build --with github.com/grafana/xk6-output-clickhouse

... [INFO] Build environment ready
... [INFO] Building k6
... [INFO] Build complete: ./k6
```
You will have a `k6` binary in the current directory.

# Database configure

If custom database (not default) is used
```
CREATE DATABASE IF NOT EXISTS k6 ON CLUSTER cluster
USE k6
```

Create replicated schema
```
CREATE TABLE IF NOT EXISTS k6_samples (
    id UInt64,
    start DateTime64(9, 'UTC'),
    ts DateTime64(9, 'UTC'),
    metric String,
    url String,
	label String,
    status String,
    name String,
    tags Map(String, String),
    value Float64,
    version DateTime64(9)
) ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/k6_samples', '{replica}', version)
PARTITION BY toYYYYMM(start)
ORDER BY (id, start, ts, metric, url, label, status, name);

CREATE TABLE IF NOT EXISTS k6_tests (
    id UInt64,
    ts DateTime64(9, 'UTC'),
    name String,
    params String
) ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/k6_tests', '{replica}', id)
PARTITION BY toYYYYMM(ts)
ORDER BY (id, ts, name);
```

If no tables at start, atotomatic create database and non-replicated schema, like this

```
CREATE DATABASE IF NOT EXISTS k6
USE k6

CREATE TABLE IF NOT EXISTS k6_samples (
    id UInt64,
    start DateTime64(9, 'UTC'),
    ts DateTime64(9, 'UTC'),
    metric String,
    url String,
	label String,
    status String,
    name String,
    tags Map(String, String),
    value Float64,
    version DateTime64(9)
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(start)
ORDER BY (id, start, ts, metric, url, label, status, name);

CREATE TABLE IF NOT EXISTS k6_tests (
    id UInt64,
    ts DateTime64(9, 'UTC'),
    name String,
    params String
) ENGINE = ReplacingMergeTree(ts)
PARTITION BY toYYYYMM(ts)
ORDER BY (id, ts, name);
```

# Configuration

First, find the [Clickhouse-go DSN connection string](https://github.com/ClickHouse/clickhouse-go#databasesql-interface) of the clickhouse instance.

To run the test and send the k6 metrics to clickhouse, use the `k6 run` command setting the [k6 output option](https://k6.io/docs/using-k6/options/#results-output) as `clickhouse=YOUR_CLICKHOUSE_CONNECTION_STRING`. For example:


```bash
k6 run -o "clickhouse=http://k6:k6@localhost:8123/default?dial_timeout=200ms&max_execution_time=60" script.js
```

or set an environment variable:

```bash
K6_OUT="clickhouse=http://k6:k6@localhost:8123/default?dial_timeout=200ms&max_execution_time=60" k6 run script.js
```

For non-default database
```bash
K6_OUT="clickhouse=http://k6:k6@localhost:8123/k6?dial_timeout=200ms&max_execution_time=60" k6 run script.js
```

## Options

The `xk6-output-clickhouse` extension supports this additional option:

- `K6_OUT_CLICKHOUSE_PUSH_INTERVAL`: to define how often metrics are sent to clickhouse.  The default value is `10s` (10 second).
- `K6_OUT_CLICKHOUSE_TESTNAME`: to set test name prefix prepended to id (formated start timestamp).
