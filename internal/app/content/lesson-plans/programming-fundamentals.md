---
id: programming-fundamentals
title: Programming Fundamentals
language: agnostic
version: 1
---

## Learning goal

Learner can read, trace, and debug a 50-line program without hints. Examples and exercises are
delivered in whatever language the learner's current diff is written in.

## Prerequisites

None. Entry-level track.

## Soul mapping

| Concept | Soul |
|---|---|
| PROG-001 through PROG-008 | concepts-tutor |
| PROG-009 | debugging-coach |
| PROG-010 | code-review |

## Prerequisite sequence

PROG-001 -> PROG-002 -> PROG-003 -> PROG-004 -> PROG-005 -> PROG-006 -> PROG-007 -> PROG-008 -> PROG-009 -> PROG-010

## Concepts

### PROG-001: Values and Types
- **Objective:** States type and value of an expression before running it.
- **Diagnostic:** "What type does this expression produce, and what is its value? Don't run it yet."
- **Exercise:** Declare three variables of different types. Predict the zero/default value of each before running.
- **Misconception:** Type-inferred languages are dynamically typed. (Inference is a compile-time step; the type is still fixed.)
- **Evidence:** Correctly predicts type and value of 3 expressions without running.
- **Transfer:** Given a standard-library call, identify its return type before looking it up.
- **Prerequisites:** none

### PROG-002: Variables and Binding
- **Objective:** Explains where a variable is accessible and traces its value through assignment.
- **Diagnostic:** "What is this variable's value here? Can it be accessed from that block?"
- **Exercise:** Write code that tries to use a variable outside its declaring block. Predict whether it compiles or errors.
- **Misconception:** A variable is accessible anywhere after it is declared.
- **Evidence:** Correctly predicts compile/runtime behavior in 2 scope scenarios without running.
- **Transfer:** Spot a variable-shadowing bug in a 10-line snippet without running it.
- **Prerequisites:** PROG-001

### PROG-003: Control Flow (Conditionals)
- **Objective:** Traces the execution path of a conditional block for multiple input values.
- **Diagnostic:** "Walk me through this code assuming the input is 7. Which branch runs?"
- **Exercise:** Write a three-branch conditional. Trace the output for three different inputs before running.
- **Misconception:** The else-if and a nested if inside else always produce the same behavior.
- **Evidence:** Correctly traces execution for 3 inputs through a 3-branch conditional.
- **Transfer:** Identify a dead-code branch in a nested conditional without running it.
- **Prerequisites:** PROG-002

### PROG-004: Loops and Iteration
- **Objective:** Traces the loop variable through each iteration and states the termination condition correctly.
- **Diagnostic:** "How many times does this loop run? What is the counter equal to after it exits?"
- **Exercise:** Write a loop summing 1 through 10. Fill a trace table with the accumulator after the first 4 iterations.
- **Misconception:** The loop variable equals its last in-loop value after the loop ends. (It equals the value that failed the condition.)
- **Evidence:** Fills a 5-row trace table correctly for a counter and accumulator loop.
- **Transfer:** Identify the off-by-one error in a loop that prints one too few items without running it.
- **Prerequisites:** PROG-003

### PROG-005: Functions and Return Values
- **Objective:** Predicts what a function returns given specific inputs.
- **Diagnostic:** "What does this function return when called with these arguments? Trace it without running."
- **Exercise:** Write a function returning two values. Call it and predict both values before running.
- **Misconception:** A function can only return one thing.
- **Evidence:** Correctly predicts return values for 3 function calls, including at least one multi-value return.
- **Transfer:** Given a function that can signal failure through its return, explain what the caller must check before using the result.
- **Prerequisites:** PROG-004

### PROG-006: State Tracing
- **Objective:** Fills a complete trace table for a 10-15 line program with a loop and a conditional.
- **Diagnostic:** "Run this in your head. What is every variable equal to just before the return?"
- **Exercise:** Given a function with a loop and a conditional inside, fill a trace table by hand then verify with print statements.
- **Misconception:** Reading code top to bottom once is the same as understanding what it does.
- **Evidence:** Completes a 5-iteration trace table with at most 1 error before running.
- **Transfer:** Trace a variable's value across a function call boundary without running.
- **Prerequisites:** PROG-005

### PROG-007: Ordered Collections
- **Objective:** Predicts the result of add, slice, and length operations before running.
- **Diagnostic:** "Given this list, what does taking elements 1 through 3 produce? What is the resulting length?"
- **Exercise:** Start with a 5-element collection, add one element, take a sub-range. Predict length and first element at each step.
- **Misconception:** Taking a sub-range creates an independent copy. (Many languages share the underlying storage.)
- **Evidence:** Correctly predicts 3 collection operations including one that reveals shared-storage behavior.
- **Transfer:** Explain why mutating a sub-range may or may not affect the original depending on the language and operation.
- **Prerequisites:** PROG-006

### PROG-008: Key-Value Structures
- **Objective:** Predicts lookup results including missing keys.
- **Diagnostic:** "What does looking up a key that doesn't exist return in this language?"
- **Exercise:** Create a key-value structure, add entries, write code that checks whether a key exists before using it. Predict what happens without the check.
- **Misconception:** Looking up a missing key always raises an error. (Many languages return a zero/null value silently.)
- **Evidence:** Correctly handles existence-check patterns and predicts missing-key behavior.
- **Transfer:** Explain why a silent zero return could introduce a subtle bug in a frequency-counting program.
- **Prerequisites:** PROG-006

### PROG-009: Debugging from Evidence
- **Objective:** States a falsifiable hypothesis and designs one targeted print statement to confirm or deny it.
- **Diagnostic:** "Your function returns the wrong value. What is the very first thing you would do? Don't change any code yet."
- **Exercise:** Given a program with a deliberate off-by-one bug, find it using exactly 2 print statements. State the hypothesis first.
- **Misconception:** The fastest way to debug is to try random fixes until something works.
- **Evidence:** States a specific hypothesis, adds one targeted print, interprets output, finds the bug within 3 attempts.
- **Transfer:** Given a bug description without code, list 3 possible causes and one print that would distinguish between them.
- **Prerequisites:** PROG-007, PROG-008
- **Soul:** debugging-coach

### PROG-010: Decomposition
- **Objective:** Splits a 20-25 line main/entry function into at least two helpers with names describing their purpose.
- **Diagnostic:** "Which parts of this function could become their own function? What would you name each one?"
- **Exercise:** Given a single long function that reads, processes, and outputs, extract the processing step into a named function.
- **Misconception:** Helper functions should only be created when code is exactly duplicated.
- **Evidence:** Extracts at least 2 functions with names a reader can understand without reading the body.
- **Transfer:** Identify which parts of a longer function have distinct responsibilities and explain why coupling them could cause problems later.
- **Prerequisites:** PROG-009
- **Soul:** code-review
