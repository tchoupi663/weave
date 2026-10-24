# Report Patterns and Review Method

## Use This Reference

Read this reference for a report outline, a rewrite, or a detailed review. Select the pattern that matches what the reader must do next. Remove sections that are genuinely not applicable; do not leave empty headings.

## Common Front Matter

Use this block when the report needs configuration control or external review.

| Field | Include |
| --- | --- |
| Title | Object, event/action, and result or purpose |
| Document ID / revision | Stable identifier and revision date |
| Status | Draft, review, approved, superseded, or controlled copy |
| Scope | Included system, version, sites, dates, and exclusions |
| Audience | Operator, maintainer, engineering reviewer, safety owner, decision maker |
| Control level | STE-informed, STE-checked, or contract-compliant only when justified |
| Evidence window | Data sources and time period |
| Owners | Author, technical owner, reviewer, approver |

## Procedure or Work Instruction

Use only for a reader who must perform a task. Do not embed an investigation narrative inside the steps.

```text
Purpose
Applicability and limits
Required competence and authorization
References and controlled terminology
Safety information
Tools, materials, and parts
Prerequisites
Procedure
  1. Condition / stop criterion
  2. Action
  3. Expected result
  4. Failure or decision path
Closeout
Required records
```

### Step design

Use a short condition-action-result pattern.

```text
If [measurable condition], [action].
Expected result: [observable state/value].
If the result does not occur, [safe stop/escalation action].
```

Do not give the reader a condition only after the irreversible action. Put tool settings, torque/value limits, and named reference figures in the same step or immediately before it.

## Test or Validation Report

Use this structure to make a verdict auditable.

```text
1. Objective and acceptance criteria
2. Item under test (identifier, version, configuration)
3. Test environment (hardware, software, data, instruments, calibration status)
4. Method and deviations from method
5. Results by criterion
6. Anomalies, retests, and exclusions
7. Conclusion (pass/fail/inconclusive) and scope of conclusion
8. Required actions and verification plan
9. Evidence index (logs, data, screenshots, records)
```

### Results table

| Criterion ID | Method/evidence | Expected result | Observed result | Verdict | Limitation / next action |
| --- | --- | --- | --- | --- | --- |
| AC-01 | Test T-17, log L-17 | Restart count = 0 in 60 min | 3 restarts in 30 min | Fail | Root cause not isolated; perform diagnostic D-04 |

State `inconclusive` if the test cannot measure the criterion. Do not convert incomplete evidence into a pass or fail.

## Incident or Diagnostic Report

Use a fact-led timeline. A timeline makes later corrections less disruptive and stops an inference from being mistaken for an event.

```text
Current status and reader action
Scope and service/product impact
Known timeline (each event source-labeled)
Observed evidence
Analysis and hypotheses
Containment actions
Root-cause status and confidence
Corrective and preventive actions
Open questions and owners
Appendix: evidence index and terminology ledger
```

### Timeline table

| Time and zone | Event | Source | Confidence | Interpretation boundary |
| --- | --- | --- | --- | --- |
| 14:06 UTC | Service process stopped | Log L-22 | High | Event observed; cause not identified |
| 14:09 UTC | Health check failed | Monitor M-04 | High | Consequence of the stop is possible; verify relationship |

Write the next action in the current-status section. A reader who has five minutes must not read the whole report to find containment or escalation instructions.

## Engineering Assessment or Decision Report

Use this structure for a choice between options. Make the decision logic visible, not just the recommendation.

```text
Question and decision owner
Context, system boundary, and constraints
Options considered, including “do nothing” where relevant
Evaluation criteria and weights/priority
Evidence and assumptions for each option
Comparison and sensitivity/uncertainty
Recommendation and rationale
Risks, dependencies, cost, and reversibility
Decision record and verification plan
```

### Comparison table

| Criterion | Option A | Option B | Evidence | Uncertainty | Decision effect |
| --- | --- | --- | --- | --- | --- |
| Recovery time | 15 min target | 60 min target | Test T-31 | Medium | Favors A if target is mandatory |

Do not say an option is “best” without naming the criteria, their priority, and the evidence. Do not describe a preference as an engineering fact.

## Descriptive Technical Report

Use this pattern to explain a system or component without instructing the reader to operate it.

```text
Purpose and boundary
Configuration / variants
Major components and interfaces
Normal operation and states
Inputs, outputs, limits, and failure behavior
Dependencies and maintenance implications
Known limitations and references
```

Use figures and tables for relationships with several components or conditions. Make each figure or table answer a reader question; do not duplicate a dense paragraph in a diagram.

## Rewrite Method

### 1. Preserve the technical proposition

Write a short, private proposition before editing:

```text
Actor: operator
Condition: inlet temperature is more than 70 °C
Action: stop pump P-12
Reason: prevent damage to seal S-4
Evidence/result: pump status indicates STOP
```

Check the proposition against the source. Then draft for the reader. This prevents “simplification” from deleting a condition, value, or safety effect.

### 2. Split logic at the first ambiguity

Weak source:

> In the event that the pump is running abnormally, the operator should, after ensuring that there are no outstanding work orders, proceed to shut it down as soon as practicable and notify Maintenance.

STE-informed rewrite:

```text
If pump P-12 operates outside the limits in Table 3, stop the pump.
Before you stop the pump, check the active work-order list.
If an active work order requires P-12, contact the maintenance supervisor before you stop the pump.
Record the pump status and the time of the stop.
Notify Maintenance.
```

Verify that `operates outside the limits`, `active work order`, `maintenance supervisor`, and any technical verb are approved project terminology before a formal STE claim.

### 3. Make evidence scope explicit

Weak source:

> The deployment resolved the availability issue.

STE-informed rewrite:

```text
After Deployment D-27, the availability monitor showed 99.95% availability from 08:00 to 20:00 UTC.
The monitor showed 96.10% availability in the previous 12-hour window.
The report did not test other causes of low availability.
The data supports Deployment D-27 as a mitigation in this window. It does not identify the root cause.
```

### 4. Clarify a vague requirement

Weak source:

> Check the unit regularly and make sure that it is functioning correctly.

Controlled rewrite:

```text
At the start of each shift, inspect indicator L-3.
The indicator must show green.
If the indicator does not show green, do not use the unit. Report the condition to the shift supervisor.
```

Replace the interval, indicator, expected state, owner, and stop condition with approved project values. Do not invent them.

## Risk-Based Review Method

Review from highest operational risk to lowest stylistic risk.

| Priority | Finding type | Examples | Required response |
| --- | --- | --- |
| P0 | Immediate safety or operation risk | Missing prohibition, wrong value/unit, hidden stop condition, unsupported mandatory action | Stop release; obtain technical/safety owner decision |
| P1 | Material meaning or evidence risk | Unsupported conclusion, ambiguous actor, missing prerequisite, wrong configuration/version | Correct before approval |
| P2 | Terminology/control risk | Unresolved technical verb, inconsistent component name, unverified dictionary status | Resolve or record an accepted exception |
| P3 | Readability/style risk | Dense sentence, unneeded synonym, weak heading, minor repetition | Correct when it improves comprehension without scope change |

### Finding format

```text
ID: P1-03
Location: Section 4, step 6
Issue: “It” can refer to the controller or the pump.
Risk: The reader can stop the wrong item.
Rewrite: “Stop pump P-12.”
Evidence/approval needed: Confirm P-12 identifier in Drawing D-104.
Status: Open
Owner: Mechanical engineering
```

## Final Self-Review Questions

Ask these questions before delivery:

1. What must the reader know, decide, or do after reading this report?
2. Can a reader find that information in the first screen or first page?
3. Does every action have an actor, object, condition, result, and failure route when needed?
4. Which statements are facts, and can each be traced to evidence?
5. Which words contain specialized, ambiguous, or contract-controlled meaning?
6. Did any rewrite change scope, authority, unit, timing, condition, or safety effect?
7. What still needs an official Issue 9, term-base, technical, safety, legal, or operational-owner check?
