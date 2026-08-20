---
id: devops-and-infrastructure
title: DevOps and Infrastructure
language: agnostic
version: 1
status: stub — deferred until programming-fundamentals is validated
---

## Learning goal

Learner can design and operate CI/CD pipelines, containerized workloads, and cloud-native
infrastructure with measurable reliability targets.

## Prerequisites

backend-engineering (demonstrated), architecture recommended

## Soul mapping

| Area | Soul |
|---|---|
| Concepts and mental models | concepts-tutor |
| Diagnosing operational failures | debugging-coach |
| Infrastructure design | systems-tutor (deferred) |

## Concept areas (concept-level entries deferred — pending first track validation)

### CI/CD and Branching
- Pipeline stages: build → test → scan → artifact → deploy; same immutable artifact promotes
- Trunk-based vs. Gitflow: DORA correlates trunk-based with elite delivery performance
- Blue-green vs. canary: instant rollback vs. limited blast radius; different risk/cost
- Infrastructure as code: idempotency and drift detection

### Containers
- Container primitives: namespaces isolate what a process sees; cgroups limit what it can use
- Image hygiene: minimal base, pinned versions, multi-stage builds, no secrets in layers

### Kubernetes
- Pod: ephemeral unit; never treat as a pet
- Deployment: reconciles actual→desired continuously
- Service: stable virtual IP decoupling callers from pod churn
- Ingress: L7 HTTP routing and TLS termination
- ConfigMap and Secret: externalized config; Secrets are base64 not encrypted
- HPA: reacts on delay; not a substitute for capacity planning
- Resource requests vs. limits: memory limits yes; CPU limits often harmful (cgroup throttling)
- Liveness vs. readiness probes: restart vs. remove-from-service; conflation causes outages

### Config, Secrets and Operations
- Secrets management: dynamic short-lived credentials over static long-lived ones
- SLO/SLI/error budget: error budget turns reliability into a currency governing release pace
- On-call culture: page only on user-impacting, actionable signals; alert fatigue is a reliability risk
- Postmortem: blameless, timeline-based, actions tracked to completion
- Chaos engineering: find weaknesses before an outage does

### Observability and Performance
- Logs vs. metrics vs. traces (see architecture track)
- Tail latency / p99: averages lie; design for the tail
- Profiling and flame graphs: measure before optimizing; intuition about hotspots is usually wrong
- DORA metrics: deployment frequency, lead time, change failure rate, time-to-restore
