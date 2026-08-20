---
id: systems-and-concurrency
title: Systems and Concurrency
language: agnostic
version: 1
status: stub — deferred until programming-fundamentals is validated
---

## Learning goal

Learner can reason about concurrent execution, shared state hazards, synchronization primitives,
and inter-process communication without producing data races or deadlocks.

## Prerequisites

programming-fundamentals (demonstrated), backend-engineering recommended

## Soul mapping

| Area | Soul |
|---|---|
| Concepts and mental models | concepts-tutor |
| Diagnosing races and deadlocks | debugging-coach |
| System-level design | systems-tutor (deferred) |

## Concept areas (concept-level entries deferred — pending first track validation)

### Concurrency Primitives
- Mutex: ownership, lock granularity, never hold across I/O
- Semaphore: counting gate with no ownership; right for resource pools
- Spinlock: busy-wait; correct only for very short critical sections on multicore
- Condition variable: predicate re-check in loop; lost-wakeup race

### Concurrency Hazards
- Race conditions: invisible in testing, appear under production interleavings
- Deadlock: four Coffman conditions and prevention levers (global lock ordering)
- Livelock: active but no progress; randomized backoff
- Starvation: fairness policies trading throughput for bounded latency

### Concurrency Models
- Actor model: shared state via messages; trades memory races for distributed-systems problems
- Async vs. multithreading: async is concurrency not parallelism; blocking an event loop on CPU work
- Goroutines / green threads: M:N scheduling; cheap but not magically parallel
- Event loops: one blocking task stalls everything
- Memory models: happens-before; why "it worked on my machine" concurrency is illusory
- Lock-free data structures: CAS, ABA problem, memory reclamation

### Execution Units and IPC
- Process vs. thread vs. coroutine: isolation/cost tradeoffs
- Pipes: byte-stream, unidirectional, composable
- Sockets: bidirectional; Unix domain vs. network
- Shared memory: fastest IPC; you own all synchronization
- Signals: async notifications with severe async-signal-safe constraints

### Advanced
- False sharing: independent variables on the same cache line
- Structured concurrency: scope tasks to a lexical block; no orphaned tasks
- Async cancellation: harder than starting; leaked tasks as a bug class
- Amdahl's and Little's Law: serial fraction caps speedup; L = λW for pool sizing
