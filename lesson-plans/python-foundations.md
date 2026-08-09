---
id: python-foundations
title: Python Foundations
language: python
version: 1
---

## Learning goal

Learner can independently design, test, and explain a small Python command-line application.

## Prerequisites

None. Entry-level track.

## Outcomes

Graduates can:
1. Read and trace unfamiliar code
2. Break a problem into functions and data
3. Use Python's core types and standard library
4. Debug from evidence instead of guessing
5. Write focused automated tests
6. Reason about basic time and space complexity
7. Build and document a small command-line application

## Concepts

| ID | Topic | Learner builds | Completion gate |
|---|---|---|---|
| PY-000 | Editor and execution | `hello.py` | Run, edit, and explain a Python program |
| PY-001 | Values, types, and variables | Unit converter | Predict expression values and explain type errors |
| PY-002 | Conditions and Boolean logic | Ticket-price calculator | Cover every branch with chosen inputs |
| PY-003 | Loops and iteration | Number guessing game | Explain termination and remove duplicated work |
| PY-004 | Functions | Text statistics tool | Decompose behavior into small functions |
| PY-005 | Strings and collections | Contact search | Choose correctly among list, tuple, set, and dict |
| PY-006 | Files and structured data | JSON-backed task list | Preserve data across runs and handle missing input |
| PY-007 | Errors and debugging | Repair a broken importer | Reproduce, isolate, explain, and fix each failure |
| PY-008 | Modules and dependencies | Split the task list | Keep boundaries clear; use stdlib before packages |
| PY-009 | Testing | Test the task-list core | Cover normal, boundary, and failure cases |
| PY-010 | Objects and data models | Model tasks with `dataclass` | Explain when an object beats functions + dicts |
| PY-011 | Algorithms and complexity | Search and ranking tools | Compare alternatives; state time and space costs |
| PY-012 | Refactoring | Improve a tangled program | Preserve behavior while reducing coupling |
| PY-013 | Capstone | Complete CLI application | Tests pass; invalid input handled; design defended |

## Lesson contract (each concept)

```
objective     one observable skill
concepts      at most three new ideas
starter       runnable code or a concrete task
questions     prediction -> evidence -> explanation
exercise      one guided and one independent change
check         command producing an objective pass/fail result
reflection    one question connecting to prior work
```

## Milestones

- After PY-004: repair a small unfamiliar program
- After PY-008: repair a small unfamiliar program
- After PY-012: repair a small unfamiliar program

## Capstone options

- Personal expense tracker
- Study flashcard system
- Local bookmark manager
- Habit tracker

Required: persistent JSON/CSV storage, at least four user operations, input validation, automated tests for core behavior, helpful errors without data loss.

## Assessment weights

Functionality 40%, correctness and error handling 25%, tests 20%, learner explanation 15%. No grades based on speed.
