---
id: backend-engineering
title: Backend Engineering
language: agnostic
version: 1
status: stub — deferred until programming-fundamentals is validated
---

## Learning goal

Learner can design and reason about server-side systems: clean code principles, API design,
data persistence, authentication, caching, and message queues.

## Prerequisites

programming-fundamentals (demonstrated)

## Soul mapping

| Area | Soul |
|---|---|
| Clean code, SOLID, patterns | code-review |
| API design, auth, caching, database | concepts-tutor |
| Debugging production behavior | debugging-coach |
| Architecture decisions | architecture-tutor (deferred) |
| Backend systems depth | backend-engineer (deferred) |

## Concept areas (concept-level entries deferred — pending first track validation)

### Clean Code
- Naming by intent and level of abstraction
- Single responsibility and cohesion
- Function design: command/query separation, single level of abstraction
- Error handling as a first-class contract
- Wrapping third-party code at boundaries

### SOLID Principles
- Single Responsibility: responsible to one actor, not "does one thing"
- Open/Closed via polymorphism, not speculative abstraction
- Liskov Substitution: behavioral subtyping (pre/postconditions)
- Interface Segregation: fat interfaces force unnecessary dependencies
- Dependency Inversion: high-level policy depends on abstractions

### Design Patterns
- Factory, Observer, Strategy, Decorator, Adapter
- Repository and Unit of Work
- Saga and Outbox for distributed writes
- CQRS and Event Sourcing
- Singleton as anti-pattern

### Domain-Driven Design
- Aggregates as transactional consistency boundaries
- Bounded contexts and translation at seams
- Value objects and immutability
- Domain events as decoupling mechanism

### API Design
- REST constraints vs. RPC-over-HTTP
- Idempotency and idempotency keys
- Pagination: cursor/keyset over offset
- Versioning: additive evolution over version bumps
- Contract testing

### Authentication and Authorization
- JWT: signed not encrypted; revocation problem
- OAuth 2.1 / RFC 9700: PKCE mandatory, Implicit removed
- RBAC vs ABAC tradeoffs
- Session vs token: when each is correct

### Caching
- Write-through, write-behind, cache-aside patterns
- TTL as staleness budget; jitter to prevent stampedes
- Cache invalidation as the genuinely hard problem

### Database
- Normalization and deliberate denormalization
- Indexing: composite order, covering indexes, selectivity
- N+1 queries and ORM emission
- Transactions: scope to consistency boundary
- ACID and isolation levels; write skew under snapshot isolation
- Query planning and EXPLAIN ANALYZE

### Message Queues
- At-least-once vs effectively-once (true exactly-once is impossible)
- Dead letter queues: bounded retries, alerting, ownership
- Backpressure: retries without it amplify failure
- Ordering: per-partition-key, not global
