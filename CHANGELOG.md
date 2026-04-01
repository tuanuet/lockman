# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [1.1.0] - 2026-03-31

- Add Phase 3c observability and inspection support:
  - `observe` package: event model, bounded async dispatcher, OTel adapter
  - `inspect` package: in-memory store, admin HTTP handlers, SSE streaming
  - Additive `WithBridge()` options for runtime and worker managers
  - Root SDK `WithObservability()` wiring with process-local state
- Backward-compatible: existing callers compile unchanged

## [1.0.0] - 2026-03-30

- Release the stable `github.com/tuanuet/lockman` root SDK module.
- Publish the user-first `Run` and `Claim` SDK path with example-driven docs and adapter modules.
