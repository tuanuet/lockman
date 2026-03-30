# 2026-03-30 Release Hygiene

## Goal

Bring the repository metadata and release state in line with the published `v1.0.0` release.

## Scope

- remove stale pre-release messaging from the root public docs
- add missing repository-level release artifacts
- normalize nested-module version tags for the released adapter modules

## Files

- Modify: `README.md`
- Modify: `doc.go`
- Create: `CHANGELOG.md`
- Create: `LICENSE`
- Create: `release_hygiene_test.go`

## Steps

- [ ] Write a failing test that asserts the root package docs and README no longer describe the project as pre-release or under construction, and that repository-level release files exist.
- [ ] Run the targeted test to confirm the current repository fails for the expected reasons.
- [ ] Make the minimum documentation and repository-file changes needed to satisfy the test.
- [ ] Re-run the targeted test, then run the full Go test matrix already used by CI.
- [ ] Create nested-module `v1.0.0` tags for:
  - `backend/redis`
  - `idempotency/redis`
  - `guard/postgres`
- [ ] Verify local tags exist and record that they still need to be pushed to the remote for Go module consumers.
