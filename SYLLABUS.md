# Programming Foundations — Syllabus

## Goal

Take a learner from no programming experience to independently designing, testing, and explaining a small Python application.

## Format

- 14 modules, completed at the learner's pace
- Each module: explanation, prediction, guided exercise, independent exercise, reflection
- The tutor asks one question at a time and evaluates reasoning, not exact wording
- After two failed attempts, it gives a smaller hint; after four, it shows an unrelated example
- A module is complete only when its independent exercise passes and the learner can explain the result

## Outcomes

Graduates can:

1. Read and trace unfamiliar code.
2. Break a problem into functions and data.
3. Use Python's core types and standard library.
4. Debug from evidence instead of guessing.
5. Write focused automated tests.
6. Reason about basic time and space complexity.
7. Build and document a small command-line application.

## Modules

| # | Topic | Learner builds | Completion gate |
|---|---|---|---|
| 0 | Editor and execution | `hello.py` | Run, edit, and explain a Python program from Zed or Neovim |
| 1 | Values, types, and variables | Unit converter | Predict expression values and explain type errors |
| 2 | Conditions and Boolean logic | Ticket-price calculator | Cover every branch with chosen inputs |
| 3 | Loops and iteration | Number guessing game | Explain termination and remove duplicated work |
| 4 | Functions | Text statistics tool | Decompose behavior into small functions with clear inputs and outputs |
| 5 | Strings and collections | Contact search | Choose correctly among list, tuple, set, and dictionary |
| 6 | Files and structured data | JSON-backed task list | Preserve data across runs and handle missing or malformed input |
| 7 | Errors and debugging | Repair a broken importer | Reproduce, isolate, explain, and fix each failure |
| 8 | Modules and dependencies | Split the task list into modules | Keep boundaries clear and use the standard library before packages |
| 9 | Testing | Test the task-list core | Write tests for normal, boundary, and failure cases |
| 10 | Objects and data models | Model tasks with `dataclass` | Explain when an object is clearer than functions plus dictionaries |
| 11 | Algorithms and complexity | Search and ranking tools | Compare alternatives and state their time and space costs |
| 12 | Refactoring and clean code | Improve an intentionally tangled program | Preserve behavior while reducing duplication and coupling |
| 13 | Capstone | Complete CLI application | Pass tests, handle invalid input, and defend design decisions |

## Capstone

Build one of:

- Personal expense tracker
- Study flashcard system
- Local bookmark manager
- Habit tracker

Required behavior:

- Persistent JSON or CSV storage
- At least four user operations
- Validation at every input boundary
- Automated tests for core behavior
- Helpful errors without data loss
- README with setup, usage, and design trade-offs

The tutor may question and hint, but it does not write capstone code for the learner.

## Lesson Contract

Every lesson must define:

```text
objective     one observable skill
concepts      at most three new ideas
starter       runnable code or a concrete task
questions     prediction → evidence → explanation
exercise      one guided and one independent change
check         command producing an objective pass/fail result
reflection    one question connecting the lesson to prior work
```

## Assessment

- **Module checks:** executable checks plus a short explanation in the learner's own words
- **Milestones:** after modules 4, 8, and 12, repair a small unfamiliar program
- **Capstone:** functionality 40%, correctness and errors 25%, tests 20%, explanation 15%

No grades are based on speed or tokens used. Progress is based on demonstrated understanding.

## Built-in Course Scope

Version 1 ships only this Python foundations course. Additional languages reuse its learning outcomes after the course format and tutor behavior work in practice.
