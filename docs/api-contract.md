# API contract v1

Base path: `/api/v1`. JSON timestamps are UTC RFC3339 strings (prefer nanosecond precision). IDs are opaque strings; seeded agent IDs are `premier_league`, `bundesliga`, and `coach_statements`. Lists are oldest-first for messages and stable `id` order for agents. Pagination is not yet needed for these MVP reads; future list endpoints will use explicit cursor fields rather than silently changing ordering.

Enums: run status `queued | running | completed | failed`; draft review status `pending | approved | rejected`; claim status `official | reported | unverified`; message role `user | assistant | system`. Claim status describes evidence attribution, not a guarantee of truth.

## Working endpoints

`GET /health` → `200`

```json
{"status":"ok"}
```

`GET /api/v1/agents` → `200`

```json
{"agents":[{"id":"premier_league","assignment":"Premier League news","language":"fr","platforms":["facebook","x"],"enabled":true,"researchIntervalSeconds":1800,"pendingDraftCount":0,"isRunning":false,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]}
```

`GET /api/v1/agents/{id}/messages` → `200`. A valid new conversation has no fabricated content and returns an empty list:

```json
{"messages":[]}
```

If messages exist, the response is:

```json
{"messages":[{"id":"message-1","agentId":"premier_league","role":"assistant","messageType":"draft","text":"Texte…","draftId":"draft-1","runId":null,"createdAt":"2026-01-01T00:00:00Z"}]}
```

For a persisted draft message, an additive `draft` object is included with `id`, `agentId`, `storyId`, `runId`, `headline`, `claimStatus`, `facebookText`, `xText`, `reviewStatus`, `sources`, `createdAt`, and `updatedAt`. Sources have `id`, `url`, `title`, nullable `publishedAt`, and `retrievedAt`. Nullable fields are emitted as JSON `null`.

An unknown agent returns `404`:

```json
{"error":{"code":"not_found","message":"Agent not found."}}
```

## Manual research run

`POST /api/v1/agents/{id}/runs` accepts an empty JSON body (or no body) and returns `202` after persisting a queued run:

```json
{"run":{"id":"run_abc123","agentId":"premier_league","status":"queued","startedAt":"2026-01-01T00:00:00Z","endedAt":null,"error":null}}
```

Only one queued or running run is allowed per agent. An active assignment returns `409` with `code: "conflict"`. If credentials are missing, no run is created and the response is `503` with `code: "configuration_error"`. A full bounded queue returns `503` with `code: "queue_full"`. The frontend should poll agent list state and `GET /agents/{id}/messages`; successful nothing-new runs append a concise assistant status message and create no draft.

## Deferred mutations

`POST /api/v1/agents/{id}/messages` and `PATCH /api/v1/drafts/{id}` currently return `501`:

```json
{"error":{"code":"not_implemented","message":"This operation is not implemented yet."}}
```

Intended deferred behavior: posting `{ "text":"Corrige ce titre" }` persists a user message and may initiate a revision. Draft patch accepts any of `{ "facebookText":"..." }`, `{ "xText":"..." }`, `{ "reviewStatus":"approved" }`, or rejected status; approval only changes internal review state and never publishes.

Errors consistently use `{ "error": { "code": "...", "message": "..." } }`; common codes are `not_found` (404), `invalid_request` (400), `conflict` (409), `not_implemented` (501), and `internal_error` (500).

Pending draft counts and running state are derived from `drafts.review_status` and `runs.status`; they are not redundant stored counters.
