# Production Hardening Audit Remediation TODO

Last updated: 2026-05-25

## Current State

- Branch: `codex/production-hardening-audit-remediation`
- Working tree at handoff: clean before this TODO file was added.
- Latest implementation commit: `8e344cf fix(bridge): bound and validate external invocations`
- Original plan: `docs/superpowers/plans/2026-05-25-production-hardening-audit-remediation.md`
- Human login requirement: keep passwordless bare handle login. It is intentional convenience identity, not a bug to remove.

## Completed

- Task 1 HTTP lifecycle, health, stats auth.
  - Commits: `d149e4c`, `d04797d`, `3924d72`
  - Reviewed by spec and code-quality reviewers; approved.
- Task 2 Group authorization and invite atomicity.
  - Commits: `70f3803`, `a2ad75f`, `87afa7f`
  - Reviewed by spec and code-quality reviewers; approved.
- Task 3 Store safety, registry ordering, migrations.
  - Commits: `0e7b954`, `b66b547`
  - Reviewed by spec and code-quality reviewers; approved.
- Task 4 LLM stream safety.
  - Commits: `2232b17`, `9f6ef65`, `51e50b5`
  - Reviewed by spec and code-quality reviewers; approved.
- Task 5 Engine and tool context propagation.
  - Commits: `73d9cc5`, `b9297e5`, `397cff0`
  - Reviewed by spec and code-quality reviewers; approved.
- Task 6 Bridge HTTP and CLI safety.
  - Commit: `8e344cf`
  - Spec review passed.
  - Code-quality review was started but stopped for handoff; do this next.

## Remaining TODO

1. Finish Task 6 code-quality review.
   - Review diff `397cff0..8e344cf`.
   - Focus on `internal/bridge/http.go`, `internal/bridge/cli.go`, `internal/bridge/bridge_test.go`, and `docs/architecture/current-architecture.html`.
   - Verify no hidden regression in URL validation, dedicated HTTP client timeout, CLI stdout bounding, or docs wording.
   - If issues are found, fix and commit before continuing.

2. Do Task 7: correct audit and reporting docs.
   - Update `docs/audit-report.md`.
   - Update `docs/architecture/current-architecture.html`.
   - Update `docs/production-readiness/acceptance-matrix.md`.
   - Required corrections:
     - Project size should be corrected to 53 Go files and 14 Go test files as of 2026-05-25.
     - Human handle login should be reclassified from Critical vulnerability to documented passwordless convenience identity.
     - Group member deletion should say valid same-group member tokens could delete other same-group members.
     - Panic recovery finding should be scoped to production code; tests may contain recover.
     - `ConfigureAuxiliaryAgentTools` finding should be scoped to DB-loaded builtin agents only.
     - TaskItem claim finding should be corrected to missing `RowsAffected` and non-transactional dependency check.
   - Run stale-text check:
     ```bash
     rg -n "38 源文件|Handle-only login 无凭证验证|recover\\(\\) 出现次数为 \\*\\*0\\*\\*|仅对第一个 DB agent" docs/audit-report.md docs/architecture/current-architecture.html docs/production-readiness/acceptance-matrix.md
     ```
     Expected: no output.
   - Commit message: `docs: reconcile audit remediation status`.

3. Do Task 8: full verification.
   - Run:
     ```bash
     go test ./...
     go test -race ./internal/svc ./internal/events
     git status --short
     ```
   - Expected: tests pass and working tree clean.
   - Residual risks to mention in final response:
     - Passwordless handle login is intentionally convenient, not strong authentication.
     - Bridge CLI shell snippets remain operator-trusted config; strict argument safety depends on config style.

4. Final whole-branch review.
   - Review all remediation commits from `14af3e6..HEAD`.
   - Confirm architecture doc only describes current implementation and labels intentional/deferred behavior clearly.
   - Confirm no code path accidentally removed bare handle login.

## Quick Resume Commands

```bash
cd /Users/lijialun/work/a2a-platform-go
git status --short --branch
git log --oneline -n 18
go test ./internal/bridge -count=1
```
