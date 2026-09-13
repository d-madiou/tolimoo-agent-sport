# Architecture

The single Go process exposes `net/http`, stores shared newsroom data in SQLite, and hosts scheduler and provider adapters. Expo Router is a thin client that consumes the stable contract through `src/services/api.ts`; `mock-api.ts` is an explicit local substitute.

Workflow: when enabled, the scheduler queues an assignment run through the same bounded worker used by manual runs → Exa researches developments → stories are deduplicated into shared history → OpenRouter produces a sourced French draft for Facebook/X → the editor reviews, edits, approves, or rejects → approval changes only internal state and never publishes externally. SQLite stores each enabled assignment's next due time; first automatic attempts are one interval ahead and completed, nothing-new, and failed runs all move the next attempt forward by that interval.

Stories are shared across agents so repeated findings can be deduplicated. Drafts belong to an agent and run; messages belong to an agent conversation (one conversation per agent for this MVP).

Implemented now: numbered SQL migration file execution with a repeatable migration ledger, seed data, health, agent listing with derived pending/running state, conversation reading, unknown-agent errors, request logging/recovery, CORS, timeouts, body limit, graceful shutdown, and a manual one-worker research queue. The queue persists queued runs before returning, recovers interrupted runs on restart, and atomically prevents more than one active run per agent.

The manual workflow begins with the current UTC date and a seven-day news window, uses at most two Exa searches (five results each), asks OpenRouter first to assess evidence and optionally request one follow-up search, then requests final structured French drafts. Source text is bounded and treated as untrusted data. The model can only cite backend-assigned source IDs, which are validated before persistence; official-source IDs must also be among that draft's own evidence. Final stories, sources, drafts, messages, and the completed run state are saved in one transaction. Exact normalized source URLs and recent story summaries provide basic deduplication; this is not semantic-deduplication accuracy.

Planned: editorial message/revision mutations and publishing integrations.
