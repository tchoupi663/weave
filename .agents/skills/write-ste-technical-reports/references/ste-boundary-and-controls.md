# ASD-STE100 Boundary and Controls

## Purpose of This Reference

Use this reference when a request mentions ASD-STE100, compliance, Simplified Technical English, a controlled dictionary, official terminology, or a safety-critical report. It summarizes public information only. It does not reproduce the official standard and cannot substitute for it.

## Public Baseline

The ASD Simplified Technical English Maintenance Group describes ASD-STE100 as an international standard for technical documentation in a controlled natural language. The current published issue is Issue 9, dated January 15, 2025. The official public material says that the standard has:

- 53 writing rules in nine sections;
- a controlled dictionary with about 900 approved general words and about 1,200 non-approved words with alternatives;
- one intended meaning and one part of speech for a general approved word in most cases; and
- provisions for technical nouns and technical verbs that belong to a company, industry, or subject field.

Public guidance also states that the standard uses American English and the Merriam-Webster dictionary as its basis for approved general words and spelling. It says that procedures use direct imperative language and that the active voice is preferred. It notes that `-ing` forms are usually restricted because they can be ambiguous.

Do not extrapolate this summary into an exhaustive rule list. The official Issue 9 PDF is the controlling source. Its official download page offers a free copy by request.

## Required Compliance Language

Use one of these statements exactly or adapt it without strengthening the claim.

| Situation | Safe statement |
| --- | --- |
| No official standard/dictionary available | “This report applies ASD-STE100-informed clarity principles. It has not received a full ASD-STE100 Issue 9 dictionary and rule check.” |
| Official standard and term base available; review complete | “This report was checked against ASD-STE100 Issue 9 and the approved terminology listed in the compliance ledger.” |
| Tool output only | “A language tool flagged possible controlled-language issues. A qualified reviewer must check the official rule and dictionary status.” |
| Contract requires formal acceptance | “Compliance remains subject to the designated organizational review and acceptance process.” |

Never write `certified`, `fully compliant`, or equivalent unless the user supplies the applicable acceptance authority and evidence. The ASD FAQ says that a language tool is optional and cannot replace the standard itself.

## Terminology Control Procedure

### 1. Make a candidate list

Extract all terms that identify a component, function, state, measurement, process, mode, interface, software field, alert, or action. Include all abbreviations, hyphenated compounds, verbs that operate a system, and words with more than one common meaning.

### 2. Classify each term

| Class | Examples | Control question |
| --- | --- | --- |
| General word | `start`, `near`, `during` | Is this word approved for this exact meaning and part of speech? |
| Technical noun | `air-data computer`, `CRC register` | Does the exact noun exist in a drawing, specification, product interface, or approved term base? |
| Technical verb | `calibrate`, `deploy`, `re-image` | Is this exact action formally defined in the applicable source? |
| Proper identifier | `A320`, `SYS-221`, `0x0014` | Is the string exact and stable? |
| Abbreviation | `PBIT`, `ECU` | Is it defined at first use and unambiguous in this report? |

### 3. Record source and scope

For each candidate, record the official spelling, grammar class, definition or source context, owner, version/date, and status. Do not turn a prohibited general word into a “technical term” without an authoritative subject-field source.

### 4. Resolve exceptions

Mark an unresolved term `pending`. Give the reader an exact short definition if doing so does not create a safety issue. Do not bury it in an informal synonym. Request a terminologist, engineering owner, or STE reviewer decision before formal release.

## Controlled-Language Triage

Use these categories to decide what to investigate. The list is deliberately not a claimed replica of ASD-STE100 rules.

| Signal | Typical risk | First corrective action |
| --- | --- | --- |
| Passive construction | Actor or responsibility is hidden | Name the actor and use active voice; retain passive only when the actor is unknown or irrelevant and the formal rule permits it. |
| Multiple verbs/actions | Order and responsibility are unclear | Split into steps or a decision table. |
| Pronoun (`it`, `this`, `they`) | More than one possible reference | Repeat the controlled noun. |
| Vague qualifier | Reader cannot measure or decide | Give value, unit, method, threshold, or named condition. |
| Synonym | Terminology drift or dictionary issue | Use the controlled term consistently. |
| Phrasal/idiomatic verb | Meaning can vary across readers | Use a defined direct verb; check the official dictionary. |
| Long noun string | Object/action boundary is hard to parse | Split with a defined technical noun, a table, or short sentences. |
| `-ing` form | Grammatical role can be unclear | Rewrite as a finite verb, noun, or two sentences; check required remaining terms. |
| Nominalization | Action and actor disappear | Change `perform an inspection` to a clear approved verb only if the term base permits it. |
| Modal (`should`, `may`, `can`) | Requirement strength is unclear | State requirement, permission, capability, or recommendation explicitly. |

## Evidence Control Procedure

Apply this procedure before simplification. Clarity must not remove an evidential boundary.

1. Give each finding an ID.
2. Identify the source: log, inspection, test, calculation, ticket, drawing, interview, or external document.
3. State the source version, time, conditions, and collector where material.
4. Write the observation separately from the interpretation.
5. State the scope: what was tested, not tested, observed, and not observed.
6. Tie every recommendation to a finding, owner, acceptance criterion, and evidence of completion.

### Example

| Weak statement | Controlled evidence statement |
| --- | --- |
| `The system is unstable.` | `During Test T-17, the controller restarted three times in 30 minutes. The test log records restart times of 14:06, 14:14, and 14:29 UTC. The test did not isolate the cause.` |
| `The update probably fixed the problem.` | `After Update U-12, Test T-18 completed for 60 minutes without a restart. This result supports the update as a mitigation. It does not establish the root cause.` |

## Compliance Ledger Template

Include the ledger in a review record or report appendix when the reader needs assurance about the control process.

| ID | Control | Status | Evidence/decision | Owner | Due date |
| --- | --- | --- | --- | --- | --- |
| CL-01 | Official Issue 9 available | checked/not checked | Document ID and version | | |
| CL-02 | Approved terminology source available | checked/not checked | Term-base name and version | | |
| CL-03 | Safety/legal wording reviewed | checked/not applicable | Review record | | |
| CL-04 | Technical facts verified | checked/exception | Source list | | |
| CL-05 | Procedure executable | checked/exception | Walkthrough or test ID | | |
| CL-06 | General words and grammar checked | checked/not checked | Reviewer/tool plus manual validation | | |
| CL-07 | Technical nouns/verbs resolved | checked/exception | Terminology record | | |
| CL-08 | Residual risks and open questions visible | checked | Section/ID | | |

## Official Sources

- [ASD-STE100 home page](https://www.asd-ste100.org/) — standard identity, Issue 9 date, and ownership.
- [About ASD-STE100](https://www.asd-ste100.org/about_STE.html) — public description of rules, dictionary, terminology, and intended purpose.
- [ASD-STE100 FAQ](https://www.asd-ste100.org/STE_faq.html) — public guidance on controlled vocabulary, active/procedural language, `-ing` forms, tools, and formal checking limits.
- [Official download page](https://www.asd-ste100.org/STE_downloads.html) — request the current free official Issue 9 PDF.

Review the official source before formal release because an issue, term base, contract, or local policy can change the applicable control.
