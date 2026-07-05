# Layer taxonomy (shared)

Include this verbatim wherever a prompt needs to classify code. The **layer number is the
reading order**: lower = more "outside" = read first.

- **0 — Contract.** The shape of the outside world. API schemas (OpenAPI/GraphQL/proto),
  route/URL tables, public request/response DTOs, event/message schemas, published types.
  *If a reviewer needs to know "what does this accept and return," it's here.*
- **1 — Entrypoint / Port.** Code the outside world calls first. HTTP handlers/controllers,
  CLI commands, queue/stream consumers, cron jobs, webhooks, GraphQL resolvers.
  *The thin layer that receives a request and delegates.*
- **2 — Application / Use-case.** Orchestration of a single operation. Services, command/
  query handlers, transaction scripts. Coordinates domain + adapters; owns no business
  rules of its own beyond sequencing.
- **3 — Domain.** The business truth. Entities, value objects, aggregates, pure functions,
  invariants, domain events. No I/O.
- **4 — Adapter / Infrastructure.** How we talk to the outside. Repository implementations,
  SQL/ORM queries, DB migrations, HTTP/SDK clients, cache, filesystem, message publishers.
- **5 — Cross-cutting.** Everything that wraps or wires. Config, dependency injection,
  middleware, auth filters, logging, feature flags, shared utils.
- **6 — Tests & docs.** Unit/integration/e2e tests, fixtures, snapshots, README/docs,
  generated files.

## Tie-breakers

- Classify by **role in this change**, not by folder name. A function in `utils/` that is
  really the domain rule for pricing is layer 3, not 5.
- A file that mixes layers should be split into multiple change units, each at its own
  layer — do not average.
- When genuinely unsure between two layers, pick the **more outside** one and set
  `"layerReason"` to note the ambiguity. Reviewers recover faster from "shown too early"
  than "shown a dependency before its consumer."
- A DB migration or table/schema definition is layer 4 (adapter) even when it introduces
  the PR's central noun — foundational is not the same as outside.
- Framework-generated / vendored / lockfiles → layer 6.
