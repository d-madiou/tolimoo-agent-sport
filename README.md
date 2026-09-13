# Private AI Sports Newsroom

Mobile-first newsroom for a solo sports journalist. The backend owns persistence, API contracts, and the manually triggered Exa/OpenRouter research path; the Expo team owns the mobile UI. Scheduling and social publishing remain deferred.

## Prerequisites

- Go 1.23+
- Node.js 20+ and npm
- Expo CLI via the local project command (`npx expo start`)

## Run

```sh
cp backend/.env.example backend/.env
make run-api
curl http://localhost:8080/health

cp mobile/.env.example mobile/.env
cd mobile && npm install && npm start
```

The Go commands load local backend configuration from `.env` when started from `backend/`, or from `backend/.env` when started from the repository root. Existing process environment variables take precedence, including explicitly empty values. Expo reads `EXPO_PUBLIC_*` variables from its environment. Never put secrets in the mobile environment.

Backend configuration defaults are:

- `APP_ENV=development` — log context for the running environment.
- `HTTP_ADDR=:8080` — HTTP listen address.
- `DATABASE_PATH=./data/newsroom.db` — SQLite file path; startup never deletes an existing database.
- `CORS_ALLOWED_ORIGINS=http://localhost:8081,http://localhost:19006` — comma-separated allowed browser origins.
- `EXA_API_KEY`, `OPENROUTER_API_KEY`, `OPENROUTER_MODEL` — required together to trigger manual research; startup remains available without them.
- `OPENROUTER_MAX_OUTPUT_TOKENS=4096` — maximum completion-token budget for draft generation; token-limited completions are rejected rather than saved as partial JSON.
- `AGENT_SCHEDULER_ENABLED=false` — reserved scheduler flag; scheduling is currently disabled.
- `AGENT_RUN_TIMEOUT=2m` — total time limit for one manually triggered research run.
- `AGENT_QUEUE_CAPACITY=8` — bounded number of queued runs awaiting the single worker.

SQLite is created at `backend/data/newsroom.db`; startup applies the schema and idempotently seeds the three stable assignments. Foreign keys, WAL, busy timeout, and one-process connection limits are enabled.

## Mobile modes

Set `EXPO_PUBLIC_USE_MOCK_API=true` for clearly fictional local fixtures. The app shows a Demo data indicator, and mock review/mutation actions are session-only. Set it to `false` to use the HTTP API; failed real requests are shown as errors and never silently replaced with fixtures.

Physical phones cannot reach a computer backend through `localhost`. Set `EXPO_PUBLIC_API_BASE_URL` to the computer's reachable LAN IP (for example `http://192.168.1.20:8080`) or an HTTPS backend URL, and allow that origin in `CORS_ALLOWED_ORIGINS` where applicable.

## API status

Implemented: `GET /health`, `GET /api/v1/agents`, `GET /api/v1/agents/{id}/messages`, and `POST /api/v1/agents/{id}/runs`. A configured manual run uses Exa evidence and OpenRouter structured output, then saves sourced drafts to SQLite. It is asynchronous; poll the agent and conversation APIs after receiving `202`.

```sh
curl -X POST http://localhost:8080/api/v1/agents/premier_league/runs
curl http://localhost:8080/api/v1/agents
curl http://localhost:8080/api/v1/agents/premier_league/messages
```

Each run starts with a current UTC date and a seven-day recent-news window, is limited to two Exa searches, five results per search, three model calls, two drafts, and `AGENT_RUN_TIMEOUT`. Source URLs are normalized for basic exact-repeat detection and recent story summaries are supplied to the model; this does not guarantee semantic deduplication. X text is validated with an approximate 280-Unicode-character limit. Posting editorial messages and patching drafts remain deferred.

## Exa research check

Use this developer command to make one bounded Exa search (three results maximum). It prints only source titles, URLs, publication dates, and text lengths—never article text or credentials. It needs only `EXA_API_KEY`; OpenRouter configuration is not required.

```sh
cd backend
go run ./cmd/research-check -query "latest Premier League developments"

# Or from the repository root:
go run ./backend/cmd/research-check -query "latest Premier League developments"
```

## OpenRouter draft check

This isolated developer command makes one OpenRouter `Draft` request with one clearly fictional source. It makes no Exa request and never publishes anything. It requires `OPENROUTER_API_KEY` and `OPENROUTER_MODEL`; validated output is printed as indented JSON only when it contains one or two labeled fictional drafts.

```sh
cd backend
go run ./cmd/openrouter-check

# Or from the repository root:
go run ./backend/cmd/openrouter-check
```

See [docs/api-contract.md](docs/api-contract.md) and [docs/architecture.md](docs/architecture.md).

## Ownership and verification

Backend ownership includes Go, SQLite, APIs, migrations, scheduling, Exa, OpenRouter, and agent orchestration. The other developer owns the Expo UI and final interaction design.

```sh
make test
cd mobile && npm run typecheck
```
