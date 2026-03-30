# Phase 2a Example Guides Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add user-facing documentation that shows how to learn and adopt Phase 2a behavior through the existing runnable examples.

**Architecture:** Keep code behavior unchanged. Use the root `README.md` as the navigation index, then place a short `README.md` inside each Phase 2a example folder so users can understand purpose, prerequisites, run command, flow, and expected output without reading the Go source first.

**Tech Stack:** Markdown documentation, existing Go examples, existing example tests

---

### Task 1: Document The Example Index In The Root README

**Files:**
- Modify: `README.md`

- [ ] Add a `Phase 2a Example Guide` section grouped by use case instead of only by backend type.
- [ ] For each example, describe what capability it teaches, whether it needs Redis, and what output the user should pay attention to.
- [ ] Keep existing command coverage but remove duplication where the new guide already covers the same examples.

### Task 2: Add Local READMEs To Each Phase 2a Example

**Files:**
- Create: `examples/async-single-resource/README.md`
- Create: `examples/sync-composite-lock/README.md`
- Create: `examples/async-composite-lock/README.md`
- Create: `examples/composite-overlap-reject/README.md`
- Create: `examples/parent-child-overlap/README.md`

- [ ] Add a short purpose statement for each example.
- [ ] Document prerequisites (`memory` or `Redis`) and the exact `go run` command.
- [ ] Summarize the lock definition shape and the execution flow in plain language.
- [ ] List the important output lines and what each one proves.
- [ ] Keep each file concise and focused on user adoption, not internal implementation detail.

### Task 3: Verify Example Documentation Against Actual Behavior

**Files:**
- Verify: `README.md`
- Verify: `examples/async-single-resource/README.md`
- Verify: `examples/sync-composite-lock/README.md`
- Verify: `examples/async-composite-lock/README.md`
- Verify: `examples/composite-overlap-reject/README.md`
- Verify: `examples/parent-child-overlap/README.md`

- [ ] Run: `go test ./examples/... -v`
- [ ] Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/... -v`
- [ ] Run memory examples directly: `go run ./examples/sync-composite-lock` and `go run ./examples/parent-child-overlap`
- [ ] Run Redis examples directly: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/async-single-resource` and `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/async-composite-lock`
- [ ] Fix any doc text that does not match observed output.
