# Usage history and OAuth quota observations

The v8 usage snapshot records the provider, credential kind, completion time and
supported subscription quota windows alongside each request. Codex HTTP headers
and WebSocket quota events, and Claude unified 5-hour / 7-day utilization headers,
are normalized to percentages, window durations and absolute reset times. Missing
or unsupported signals remain absent. No extra upstream query is made per request.

`GET /v8/management/observability/usage` also exposes the latest passive quota
observation and display name of each OAuth credential. These observations are not
live probes and may be stale. Export/import includes historical request observations.

The management UI offers an inclusive custom time range in the browser's local
time zone. It attributes observed main-window increases to local requests between
observations, weighted by reported total tokens (request count if no tokens were
reported). Attribution uses all local keys before selecting a key or time range.
Each account and quota reset cycle is shown separately, with request coverage.
Model-specific additional pools are displayed as snapshots but not apportioned
across unrelated models. Missing baselines, reset transitions and negative changes
are not treated as zero consumption.

These are estimates, not billing records. External account use, concurrency,
observation timing, provider rounding, and different model/token costs affect the
result. Existing requests without observations cannot recover historical quota use.
A recorded zero increment may simply be below the provider's reporting precision.

## Persistence

Configure a writable file on a persistent volume, then restart:

```yaml
observability:
  usage:
    usage-statistics-enabled: true
    usage-history-file: /data/usage-history.json
```

The path defaults to empty (disabled); relative paths resolve against the config
file directory. Snapshots are atomically saved when changed, once per minute and
on graceful shutdown after draining the usage queue. An abrupt termination can
lose the most recent minute. A damaged history file is left intact and an error
is logged; the server continues serving with in-memory statistics.

The file uses mode 0600 and contains client API keys and request metadata, like
usage exports. It does not store upstream authorization headers or OAuth tokens.
Keep backups private. History currently retains all recorded requests; operators
should monitor file size. Disabling statistics stops new collection. Changing the
history path or enabling persistence requires a restart.
