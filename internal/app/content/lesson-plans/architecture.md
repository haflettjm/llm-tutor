---
id: architecture
title: Software Architecture
language: agnostic
version: 1
status: stub — deferred until programming-fundamentals is validated
---

## Learning goal

Learner can evaluate architectural tradeoffs, draw service boundaries, apply resilience patterns,
and make change-management decisions that age well.

## Prerequisites

backend-engineering (demonstrated)

## Soul mapping

| Area | Soul |
|---|---|
| Concepts and patterns | concepts-tutor |
| Code-level structural decisions | code-review |
| System design depth | architecture-tutor (deferred) |

## Concept areas (concept-level entries deferred — pending first track validation)

### Coupling, Cohesion and Dependencies
- Coupling and cohesion: the two forces behind all modular design
- Law of Demeter: train-wreck chains as a design smell
- Dependency inversion: infrastructure plugs into the domain, not the reverse

### Architectural Styles
- Hexagonal / Ports-and-Adapters: domain depends on nothing external
- Layered vs. clean architecture: shared goal, different prescriptiveness
- Microservices vs. monolith: operational complexity tradeoff, not a scaling silver bullet
- Service boundaries: follow business capabilities and data ownership, not technical layers
- Strangler fig: incremental replacement behind a facade

### Evolution and Change Management
- Tech debt taxonomy: deliberate-prudent is a financing tool; not all debt is equal
- Refactoring: behavior-preserving, under green tests, in small steps; never mix with features
- Feature flags: deploy/release decoupling; stale flags as debt
- Backward compatibility: expand-then-contract (parallel-change) migrations
- Versioning contracts: consumer-driven so you know what you can safely change

### Observability
- Logs vs. metrics vs. traces: metrics say that, traces say where, logs say why
- USE and RED methods: Utilization/Saturation/Errors per resource; Rate/Errors/Duration per service

### Distributed Systems
- CAP theorem: partition tolerance is not optional; real choice is C-vs-A during a partition
- PACELC: Else (no partition) still trades Latency vs. Consistency
- Consistency models: linearizability > sequential > causal > session guarantees > eventual
- Consensus (Raft/Paxos): majority quorum; prevents split-brain
- Replication: sync vs. async; replication lag and read-your-writes

### Resilience Patterns
- Circuit breaker: protects the caller; trips on slow responses too; half-open state
- Bulkhead: partition resources so one failure sinks only its compartment
- Retry with backoff: jitter is required; exponential alone causes synchronized storms
- Timeout: every remote call needs a deadline; deadline propagation across hops
- Fallback: graceful degradation; risk of masking real outages for writes
