---
id: go-fundamentals
title: Go Fundamentals
language: go
version: 1
---

## Learning goal

Learner can read, trace, and debug a 50-line Go program without hints.

## Prerequisites

None. Entry-level track.

## Soul mapping

| Concept | Soul |
|---|---|
| GO-001 through GO-008 | concepts-tutor |
| GO-009 | debugging-coach |
| GO-010 | code-review |

## Concepts

### GO-001: Values and Types
- **Objective:** States type and value of an expression before running it.
- **Diagnostic:** "If I write `x := 5 + 3`, what type is x and what is its value?"
- **Exercise:** Declare three variables of different types. Predict the zero value of each before running.
- **Misconception:** Go infers types so it must be dynamically typed. (Types are static; inference happens at compile time.)
- **Evidence:** Correctly predicts type and value of 3 expressions without running.
- **Transfer:** Given `fmt.Sprintf("%d", 42)`, identify the return type before looking it up.
- **Prerequisites:** none

### GO-002: Variables and Assignment
- **Objective:** Explains `:=` vs `var` and traces scope correctly.
- **Diagnostic:** "What is the difference between `var x int = 5` and `x := 5`? Which works outside a function?"
- **Exercise:** Write a program that tries to use a variable declared inside a function from outside it. Predict whether it compiles.
- **Misconception:** `:=` can be used anywhere. (Package-level variables require `var`.)
- **Evidence:** Correctly predicts compile vs. runtime behavior in 2 scope scenarios.
- **Transfer:** Spot a variable-shadowing bug in a 10-line snippet without running it.
- **Prerequisites:** GO-001

### GO-003: Control Flow (if/else)
- **Objective:** Traces the execution path of an if/else block for multiple input values.
- **Diagnostic:** "Walk me through this code step by step, assuming x is 7. Which line runs?"
- **Exercise:** Write an if/else for positive/negative/zero. Trace output for 5, -3, and 0 before running.
- **Misconception:** `else if` and a new `if` inside `else` always behave the same way.
- **Evidence:** Correctly traces execution for 3 different input values through a 3-branch conditional.
- **Transfer:** Identify a dead-code branch in a nested if/else without running it.
- **Prerequisites:** GO-002

### GO-004: For Loops
- **Objective:** Traces the loop variable through each iteration and states the termination condition correctly.
- **Diagnostic:** "How many times will `for i := 0; i < 5; i++` run? What is i equal to after the loop?"
- **Exercise:** Write a loop summing 1 through 10. Fill a trace table with the sum after each of the first 4 iterations.
- **Misconception:** The loop variable equals its last value inside the loop after it ends. (It equals the value that failed the condition.)
- **Evidence:** Fills a 5-row trace table correctly for a counter and accumulator loop.
- **Transfer:** Identify the off-by-one error in a loop printing 0-9 instead of 1-10 without running it.
- **Prerequisites:** GO-003

### GO-005: Functions
- **Objective:** Predicts what a function returns given specific inputs, including multiple-return functions.
- **Diagnostic:** "What does this function return when called with `add(3, 4)`? Trace it without running."
- **Exercise:** Write a function returning both sum and product of two ints. Predict both values for (2, 5) before running.
- **Misconception:** Go functions can return only one value.
- **Evidence:** Correctly predicts return values for 3 function calls, at least one using multiple returns.
- **Transfer:** Given a function returning `(result int, err error)`, explain what the caller must check before using `result`.
- **Prerequisites:** GO-004

### GO-006: State Tracing
- **Objective:** Fills a complete trace table for a 10-15 line program with a loop and a conditional.
- **Diagnostic:** "Run this in your head. Tell me what every variable equals just before the `return`."
- **Exercise:** Given a function with a loop and a conditional, fill a trace table by hand then verify with `fmt.Println`.
- **Misconception:** Reading code top to bottom once is the same as understanding what it does.
- **Evidence:** Completes a 5-iteration trace table with at most 1 error before running.
- **Transfer:** Trace a variable's value across a function call boundary without running.
- **Prerequisites:** GO-005

### GO-007: Slices
- **Objective:** Predicts the result of append, sub-slice, and len operations before running.
- **Diagnostic:** "Given `s := []int{1,2,3,4,5}`, what does `s[1:3]` produce? What is its length?"
- **Exercise:** Start with a 5-element slice, append one element, take a sub-slice. Predict length and first element at each step.
- **Misconception:** Sub-slices create a copy. (They share the backing array.)
- **Evidence:** Correctly predicts 3 slice operations including one that reveals the shared-array property.
- **Transfer:** Explain why appending to a sub-slice may or may not affect the original, depending on capacity.
- **Prerequisites:** GO-006

### GO-008: Maps
- **Objective:** Predicts map lookup results including missing keys and the two-value form.
- **Diagnostic:** "What does `m["missing"]` return if m is `map[string]int` and that key does not exist?"
- **Exercise:** Create a map, add 3 entries, write code checking if a key exists before using it. Predict what happens without the check.
- **Misconception:** Accessing a missing key panics. (It returns the zero value silently.)
- **Evidence:** Correctly handles the two-value lookup form and predicts missing-key behavior.
- **Transfer:** Explain why the silent zero return could introduce a subtle bug in a word-frequency counter.
- **Prerequisites:** GO-006

### GO-009: Debugging from Evidence
- **Objective:** States a falsifiable hypothesis and designs one `fmt.Println` to confirm or deny it.
- **Diagnostic:** "Your function returns the wrong value. What is the very first thing you would do? Don't change any code yet."
- **Exercise:** Given a 15-line program with a deliberate off-by-one bug, find it using exactly 2 `fmt.Println` statements. State the hypothesis first.
- **Misconception:** The fastest way to debug is to try random fixes until something works.
- **Evidence:** States a specific hypothesis, adds one targeted print, finds the bug within 3 attempts.
- **Transfer:** Given a bug description without the code, list 3 possible causes and one print that would distinguish between them.
- **Prerequisites:** GO-007, GO-008
- **Soul:** debugging-coach

### GO-010: Decomposition
- **Objective:** Splits a 20-25 line `main` into at least two helper functions with names describing their purpose.
- **Diagnostic:** "Which parts of this main function could become their own function? What would you name each one?"
- **Exercise:** Given a 25-line `main` that reads, processes, and prints, extract the processing step into a named function.
- **Misconception:** Functions should only be created when code is exactly duplicated.
- **Evidence:** Extracts at least 2 functions with names a reader could understand without reading the body.
- **Transfer:** Identify which parts of a 30-line main have distinct responsibilities and explain why coupling them could cause problems later.
- **Prerequisites:** GO-009
- **Soul:** code-review
