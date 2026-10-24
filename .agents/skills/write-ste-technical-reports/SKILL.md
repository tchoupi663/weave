---
name: write-ste-technical-reports
description: Draft, revise, and review English technical reports with ASD-STE100 Simplified Technical English (STE) principles. Use for engineering, maintenance, test, validation, incident, safety, configuration, and decision reports that must be precise, easy to understand, and usable by international readers. Use formal ASD-STE100 compliance only when the current official standard, controlled dictionary, and approved project terminology are available.
---

# Write ASD-STE100-Informed Technical Reports

Create reports that let a reader find a fact, understand its meaning, and take the correct action without inference. Treat clarity, technical correctness, traceability, and reader safety as one quality problem.

Use American English for a formal STE deliverable unless the governing contract specifies another approved convention. Preserve approved product names, labels, interface strings, part numbers, commands, units, code, and legally controlled wording exactly.

## Establish the Control Level

Choose and state one control level before drafting.

| Level | Use it when | Permitted claim |
| --- | --- | --- |
| **STE-informed** | The official Issue 9 dictionary or local terminology is unavailable. | “Written using ASD-STE100 principles; not formally checked for full compliance.” |
| **STE-checked** | The current official standard, dictionary, and local term list are available, and every open item is checked against them. | “Checked against ASD-STE100 Issue 9 and the named project terminology.” |
| **Contract-compliant** | A designated checker or organizational process has accepted the report. | Use only the organization’s approved compliance statement. |

Do not call a report “ASD-STE100 compliant” from general knowledge, an AI grammar check, or a public web summary. The standard has a controlled dictionary and detailed rules. Read [references/ste-boundary-and-controls.md](references/ste-boundary-and-controls.md) before making a compliance claim or accepting a terminology exception.

## Collect the Writing Brief

Before writing, identify the following. Ask concise questions only for information that changes meaning, safety, or acceptance.

1. Identify the reader, their task, their expected English level, and what can go wrong.
2. State the report type, operational purpose, decision or action required, and acceptance criteria.
3. Separate verified facts, reported observations, assumptions, interpretations, and recommendations. Obtain source IDs, dates, versions, units, and owners.
4. Identify binding material: safety text, contract wording, regulatory text, interfaces, commands, figures, drawings, and project terminology.
5. Select the control level and the available checking authority: official Issue 9, approved term base, STE checker, technical reviewer, and safety reviewer.

Do not conceal a missing fact with vague language. Mark it as unknown, not measured, pending, or out of scope. State what evidence would resolve it.

## Choose a Report Architecture

Use the smallest structure that lets the reader locate the purpose, evidence, result, limit, and next action. Do not force a procedure into an essay or an analysis into numbered instructions.

| Report type | Required backbone |
| --- | --- |
| Procedure or work instruction | Purpose; applicability; prerequisites; safety; tools/materials; ordered actions; expected result; closeout/records |
| Test or validation report | Objective; item/version; environment; method; criteria; results; anomalies; conclusion; evidence/attachments |
| Incident or diagnostic report | Scope; current status; known timeline; observed evidence; analysis separated from facts; impact; containment; corrective actions; open questions |
| Engineering assessment or decision report | Question; context; constraints; options; evaluation method; evidence; recommendation; risks; decision owner; verification plan |
| Descriptive technical report | Purpose; system boundary; configuration; operation; interfaces; limitations; maintenance or operational implications |

Read [references/report-patterns-and-reviews.md](references/report-patterns-and-reviews.md) for report templates, evidence patterns, and complete rewrite examples.

## Build a Terminology Record First

Create a small terminology record for all terms that carry technical meaning. Use one approved name for one object, state, action, interface, or measurement. Use the exact product or drawing name at first use. Define a shorter form only when it cannot confuse the reader.

| Term | Type | Approved source | Meaning or scope | Allowed grammar | Status |
| --- | --- | --- | --- | --- | --- |
| `pressure-relief valve` | Technical noun | Drawing D-104 | Valve that controls excess pressure in hydraulic line A | noun | approved/pending |
| `calibrate` | Technical verb | Project term base | Set instrument response against approved reference | verb | approved/pending |

In formal work, verify each ordinary word, technical noun, technical verb, spelling, meaning, and part of speech against the applicable sources. Do not assume that a familiar word is permitted in its intended meaning or grammatical role. Keep unapproved terms in an exceptions list until a reviewer resolves them.

## Draft in Controlled, Evidence-Led English

Apply these working constraints. They are a practical STE-informed drafting guide, not a replacement for the official rules or dictionary.

| Control | Apply it | Avoid |
| --- | --- | --- |
| One purpose per sentence | Express one fact, condition, command, result, or rationale at a time. Split compound logic. | Sentences that hide several actions, causes, or exceptions. |
| Visible actor and action | Use active voice. In a procedure, address the reader with a direct command. | Passive or impersonal wording that hides who acts. |
| Conditions before action | Put a condition first when the reader must know it before acting. | A condition placed after the action it limits. |
| Concrete reference | Name the exact component, record, value, or figure. Repeat the noun when a pronoun could have two antecedents. | “It,” “this,” “the above,” or “as appropriate” without a clear referent. |
| Direct positive instruction | State what to do, the object, and the expected endpoint. State a prohibition only when it prevents a specific hazard or error. | Double negatives, soft recommendations, and implied steps. |
| Controlled sentence shape | Prefer a short main clause. Use a list for several conditions, parts, values, or actions. | Nested clauses, long noun strings, and “respectively.” |
| Stable vocabulary | Use the same approved term for the same concept. Make a technical term explicit instead of using a vague synonym. | Synonym rotation, marketing language, idioms, phrasal verbs, and unexplained abbreviations. |
| Unambiguous time and quantity | Give the event, time zone if relevant, unit, tolerance, source, and measurement method. | “Recently,” “normal,” “high,” “quickly,” or “a few.” |

Treat `-ing` forms as a formal-check item. The public STE guidance says they are usually restricted because they can create grammatical ambiguity. Rewrite the sentence when possible, then confirm any remaining use in the official standard and term base.

### Write Procedures as Actions, Not Narratives

Give a reader a single observable action in each step. Use this sequence when applicable:

1. State the condition or safety boundary.
2. State the action and its object.
3. State the value, position, tool, or method that constrains the action.
4. State the expected result or decision point.
5. State the corrective path if the expected result does not occur.

Write `Install the cover.` rather than `The cover must be installed.` Do not use the imperative in a descriptive, historical, or evidence-only section.

### Separate Fact, Analysis, and Recommendation

Use labels and paragraph boundaries to prevent an interpretation from looking like a measurement.

| Information class | Required content |
| --- | --- |
| Fact or observation | Source, time, subject, measured or observed value, and relevant condition |
| Analysis or inference | Reasoning, inputs, limits, confidence, and alternative explanation where material |
| Recommendation or action | Owner, action, deadline or trigger, acceptance criterion, and verification evidence |

Use calibrated language. Write `The log shows that the service stopped at 14:06 UTC.` Write `The evidence supports a connection between X and Y; it does not identify the root cause.` Do not write `X caused Y` unless the evidence and scope establish causation.

### Control Safety and Warnings

Put a safety statement immediately before the action it controls. State the hazard, consequence, and preventive action when the approved safety format permits it. Do not weaken legally controlled warning text during simplification. Mark it for the designated safety/legal owner if it conflicts with the control level.

## Revise in Passes

Do not try to solve engineering accuracy, report architecture, and controlled language in one edit. Use these passes in order.

1. **Technical pass** — Confirm every claim, calculation, unit, version, interface, action, and limit against its source.
2. **Reader-task pass** — Verify that a reader can identify purpose, prerequisites, decision points, expected results, hazards, and next action.
3. **Structure pass** — Put the conclusion and operational impact where the report type requires them. Move conditions before actions. Convert dense sequences into numbered lists or tables.
4. **Language pass** — Apply the controlled drafting constraints. Remove vague modifiers, hidden actors, ambiguity, unnecessary nominalizations, and unapproved synonym changes.
5. **Terminology pass** — Reconcile every repeated term with the terminology record and check formal dictionary/part-of-speech issues when available.
6. **Independent review** — Ask a technically qualified reviewer to execute or challenge the described logic. For high-risk material, use separate technical, STE, and safety reviewers.

## Run Release Gates

Release only when every applicable gate passes or the exception is visible, owned, and accepted.

### Content and evidence

- Does the title, purpose, scope, and report type match the reader’s need?
- Can each factual claim be traced to a source, record, measurement, figure, calculation, or identified witness?
- Are assumptions, unknowns, analysis, and recommendations visibly separate from facts?
- Do values include units, tolerances, time basis, versions, and relevant conditions?
- Are diagrams, tables, attachments, identifiers, and cross-references accurate and present?

### Reader action and safety

- Can the reader identify the required decision or next action without interpreting vague language?
- Does each procedure state prerequisites, ordered actions, expected result, and failure path where needed?
- Are conditions placed before the actions they constrain?
- Are hazards, warnings, prohibitions, and stop conditions visible at the point of use?

### Controlled language

- Does each sentence contain one clear purpose and a visible actor/action where applicable?
- Are passive voice, pronouns, abbreviations, `-ing` forms, long modifiers, and synonym substitutions justified or rewritten?
- Does every technical noun and verb have an approved source or an unresolved exception record?
- Has an official Issue 9/dictionary check occurred before any formal compliance statement?

### Handoff statement

End the report or review with: control level; sources checked; terminology source; unresolved terms or exceptions; reviewers; residual risks; and required follow-up. Never hide a failed gate behind “reviewed” or “final.”

## Give Useful Review Output

For a review request, return:

1. A one-paragraph verdict: report purpose, selected control level, readiness, and major risk.
2. Findings ordered by severity: safety/meaning, evidence/traceability, procedure/structure, terminology, then style.
3. For each finding: location, original issue, why it can mislead, proposed rewrite, and whether official dictionary or terminology approval is required.
4. A revised excerpt or full report only after preserving all technical meaning and marking missing evidence.
5. A compliance ledger: `checked`, `not checked`, `not applicable`, or `requires owner decision`.

Do not invent facts, source identifiers, approval status, test results, or safety limits to make a report look complete.

## References

- Read [references/ste-boundary-and-controls.md](references/ste-boundary-and-controls.md) for official-public facts, compliance boundaries, terminology controls, and review ledger.
- Read [references/report-patterns-and-reviews.md](references/report-patterns-and-reviews.md) for detailed report templates, rewrite patterns, and a risk-based review method.
