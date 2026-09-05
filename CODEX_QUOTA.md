# Codex CLI OAuth quota display

CLIProxyAPI can deliver the quota for the credential serving a streaming Responses
request to an unmodified Codex CLI. After a successful request, run `/status` in
that CLI session to inspect the quota snapshot.

Enable filtered response headers in the proxy configuration:

```yaml
passthrough-headers: true
codex:
  stream-bootstrap-buffering: true
```

Bootstrap buffering defaults to true. Merge these settings into the existing
configuration rather than adding a second `codex` section. Header passthrough
applies to other filtered upstream response headers as well; existing security,
hop-by-hop, and CPA-reserved header filtering still applies.

An ordinary custom provider using `wire_api = "responses"` and the proxy's
`/v1` base URL can use HTTP/SSE. No client code changes or WebSocket opt-in are
needed for that path. End-to-end WebSocket clients continue to receive the
original `codex.rate_limits` events.

## Behavior

- Quota events received during the existing stream bootstrap are converted to
  `x-codex-*` response headers before the stream is handed to API handlers. This
  works with both WebSocket and HTTP/SSE upstreams.
- Percentages remain **used** percentages on the wire. Codex CLI computes the
  remaining percentage. Window durations come from upstream, so a weekly primary
  window is not mislabeled as a five-hour window.
- Reset timestamps and credits are forwarded. If only a relative reset delay is
  supplied, the proxy converts it to the absolute Unix timestamp read by Codex CLI.
- The event-to-header bridge sends the main account windows and credits only.
  Codex CLI 0.153.4 coalesces multiple header limit families into the last snapshot,
  so including additional model or code-review limits would hide the main account
  quota. The original WebSocket events and backend quota observations retain them.
- Quota snapshots are request-local. Connection reuse reads each request's new
  quota event, and failed bootstrap attempts do not contribute successful-response
  headers. There is no shared quota cache or account aggregation.

## Limits

HTTP headers cannot change after streaming begins. If the upstream emits quota
only after generated output starts, or bootstrap buffering is disabled, that
event cannot update the HTTP client's quota snapshot. Existing upstream HTTP
quota headers can still pass through. End-to-end WebSocket events can update
quota during the stream.

This feature does not implement proactive `account/rateLimits/read` for custom
API-key providers. A fresh CLI session must first receive a quota-bearing model
response. With multiple OAuth credentials, the display describes the credential
used for the latest response, not a session-pinned account or the sum of accounts.
