---
id: algorithms-and-data-structures
title: Algorithms and Data Structures
language: agnostic
version: 1
status: stub — deferred until programming-fundamentals is validated
---

## Learning goal

Learner can select the right data structure for a workload, analyze algorithmic complexity honestly,
and recognize when a problem is a known algorithm in disguise.

## Prerequisites

programming-fundamentals (demonstrated)

## Soul mapping

| Area | Soul |
|---|---|
| Concepts and mental models | concepts-tutor |
| Analysis and tradeoffs | algorithms-tutor (deferred) |
| Debugging algorithmic bugs | debugging-coach |

## Concept areas (concept-level entries deferred — pending first track validation)

### Complexity and Analysis
- Big-O/Θ/Ω: upper/tight/lower bounds on asymptotic growth — not "speed"
- Best/average/worst case: the case that matters is workload-dependent
- Amortized analysis: guarantee over a sequence, not a single operation; wrong for p99
- Space complexity: often the binding constraint before time is
- Asymptotic vs. real-world constants: O(n²) can beat O(n log n) at realistic n

### Core Data Structures
- Arrays and dynamic arrays: geometric growth, amortized O(1) append, O(n) resize spike
- Hash tables: O(1) average; adversarial keys degrade to O(n); hash flooding
- Swiss tables: SIMD-accelerated open addressing; Go maps since 1.24, Rust std since 1.36
- Self-balancing BSTs: red-black in library implementations; B-trees for disk
- LSM-trees: write amplification tradeoff; RocksDB, Cassandra
- Heaps: array-backed cache-friendly binary heap vs. pairing heap for decrease-key
- Skip lists: local pointer updates make concurrent implementations simpler than trees
- Union-find: inverse Ackermann amortized cost; effectively constant for all practical n

### Algorithm Design Paradigms
- Divide and conquer: independent subproblems parallelize
- Greedy: provably optimal only with matroid structure or valid exchange argument
- Dynamic programming: state definition is the skill, not the recurrence
- Backtracking: DFS + pruning; performance is entirely about pruning quality
- Two pointers: O(n) on sorted/monotonic data
- Binary search on the answer: when checking is faster than constructing

### Sorting and Searching
- Comparison sort lower bound: Ω(n log n) in the comparison model only
- Introsort: quicksort + heapsort fallback + insertion sort; C++ std::sort
- Timsort: adaptive merge sort exploiting existing runs; Python and Java object sort
- Radix sort: beats comparison bound by refusing to compare; faster on large integer keys
- Binary search: (low+high)/2 overflow bug lived 9 years in the JDK

### Graph Algorithms
- BFS vs. DFS: BFS for shortest unweighted paths; DFS for structure (cycles, SCCs, toposort)
- Dijkstra: non-negative weights only; greedy invariant broken by negative edges
- Bellman-Ford: handles negative weights; detects negative cycles
- Max flow / min cut duality: a wide range of problems are flow in disguise
- Eulerian vs. Hamiltonian: O(E) vs. NP-complete — the P/NP boundary made visceral

### Probabilistic and Approximate Structures
- Bloom filters: no false negatives; false-positive rate tunable by bits-per-element
- HyperLogLog: cardinality of billions of items in ~12 KB; 0.81% standard error
- Count-min sketch: frequency estimation; never undercounts
- Consistent hashing: adding/removing a node remaps only K/n keys

### Practical Application
- Choosing a structure: access pattern, cardinality, read/write ratio, memory budget — not the asymptotic table
- Wrong structure at scale: O(n) lookup in a hot loop is the most common "works in demo, melts in prod"
- Profiling before optimizing: intuition about bottlenecks is wrong more often than right
- Reduction as a skill: seeing that your problem is bipartite matching / topological sort / LSH in disguise
