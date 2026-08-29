# CLAUDE.md
This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

You are fast at code. Do not default to line-by-line human review as the quality gate. Use failing checks; continue until they pass. In a team, still keep review for who approved and who else knows the code exists.

## 1. Start small

Price the change, not the typing: data, public APIs, other services, rollback, incidents.

- Cheap to undo → one story, then something that runs. Plan shorter than the code.
- Expensive to undo → design first. Do not fiddle production.
- Do not treat a generated plan as complete. Name one thing it cannot know, then implement.
- Do not polish a spec through several passes before a line of code.
- Persist a spec only if a later task will cite it. Otherwise throw it away. If audit, contract, or a handoff needs "what did we agree?", record **why**, not a drifting how.

## 2. When stuck, pick the right reset

| Damage | Do | Do not |
|---|---|---|
| **Edits** fight each other (fix A breaks B, more rounds, urge to give up) | Stop adding work. Clean only the files this loop keeps touching. Rerun the original task. | Add prompt rules to steer it. Rewrite the whole repo. |
| **Talk** is stained (old topic leaks, corrections fail) | New session. Bring only facts the new task needs. | Say "ignore that" in the same thread. |

Rule out tool / permission / wrong-file failures before blaming messy code. If you cleaned and still spin, stop cleaning — the cause was not the mess.

## 3. Checks

- Prefer formatter, linter, types, and tests over a new instruction.
- Checks are **exit conditions**: change the code until they pass. Do not finish with a summary in place of a check. Name the mechanism: tool, a second pass in a fresh context, or a human spot-check.
- Line coverage only proves a line ran. Ask: if this broke, would a test notice?
- Calibrate numbers in **this** repo. Agreement from a model is not evidence. If the loop is slower than a human doing the same job, drop the costliest check that finds the fewest bugs.
- Green checks are not proof the design is good. Do not raise a threshold because there are too many alerts.

## 4. Structure

- Report what exists and who depends on whom. Do not re-partition modules unless asked. If asked, report the current map, propose a split, wait for confirmation, then change.
- Dependency **direction** can be a checker. On fail: invert a dependency, insert an interface, or split a module.
- Cannot name a module in one sentence → split it.
- Skip an implementation only if the name does not lie. If nobody has read the impl in a long time, do not trust the name.

## 5. Split work only when it pays

Split a job only to **shrink context** or **run in parallel**. "More specialized" is not a reason. One duty per context. The implementer may leave a mess; a **fresh** context hardens it. Hand off artifacts, not the chat. Do not split agents by job title. Do not run a pipeline on a three-line change.

## 6. Before copying a human habit

Ask: **which human limit does it compensate for?**

- Memory, fatigue, typing speed → drop the *order of work*; keep the *goal* (e.g. drop line-by-line TDD; keep "this must be tested").
- A property the code should have → keep it, preferably as a failing check.
- "A human can only hold this much" → keep the metric; re-measure the number here.

If the human is still doing the work, their habits still apply to them.


## 0. Project Identity (Pre-flight)
Before applying the rules above, load these immutable facts:
- **Tech Stack**: [Language, Framework, Package Manager, Minimum Version].
- **Entrypoints**: Where does the system start? (Main, CLI, Server init).
- **Ground Truth**: The `main`/`master` branch is the source of truth. 
  - If tests pass locally but fail in CI, treat the CI's failure as the ultimate exit condition.
  - If tests do not exist, generating a single "smoke test" that proves the entrypoint loads is mandatory before any structural changes.
- **Critical Path**: Identify the 1-2 user journeys that make or break the product. Never suggest changes that block these paths without a rollback plan.
- **Documentation location**: Where is the actual `README` or `ARCHITECTURE.md`? (Do not rely on my context history for this).