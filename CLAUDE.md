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

- **Tech Stack**: Go 1.26, std `net/http` + `html/template`, SQLite via `modernc.org/sqlite`, module `go-forum`. No CGO.
- **Entrypoints**: `cmd/forum/main.go` — load `forum.toml`, open SQLite, ensure founder, listen HTTP.
- **Ground Truth**: `main` is source of truth. Work on feature branches. Local exit condition is `go test ./...` (plus `gofmt` and `go vet`). GitHub Actions (`.github/workflows/ci.yml`) is the remote exit condition: if local is green and CI is red, CI wins.
- **Critical Path**: Logged-in member opens a board, starts a thread, replies, and sees Markdown (including http(s) image URLs) immediately. Unauthenticated visitors see only the login page.
- **Documentation**: `README.md` (how to run), `CONTEXT.md` (glossary), `docs/adr/` (hard decisions), `notes/` (config and usage notes).
