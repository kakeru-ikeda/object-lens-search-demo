# Task Plan: No-Vector Google Visual Search Implementation

## Goal
Implement the Google visual-search direction without customer-managed vector search, vector databases, embedding indexes, or Vector Search collections.

## Scope
- Preserve the current Bedrock + Tavily MVP flow.
- Keep multi-crop input and query-quality scaffolding.
- Remove Vertex AI embedding and Vector Search provider implementation.
- Keep the future path focused on Cloud Vision Web Detection, OCR, Logo Detection, Label Detection, optional Product Recognizer, and LLM/Tavily evidence synthesis.

## Phases
1. [complete] Create implementation branch `feat/google-visual-search`.
2. [complete] Add multi-crop request/response scaffolding.
3. [complete] Analyze and remove Vector Search / embedding implementation after user clarified no-vector requirement.
4. [complete] Update frontend and backend response schema to remove visualMatches/visualSearch vector-search fields.
5. [complete] Rewrite architecture document as no-vector Google visual-search design.
6. [in_progress] Verify with grep, backend tests/build/vet, frontend typecheck/build, and Oracle review.

## Decisions
- No `backend/internal/embedding/` package.
- No `backend/internal/visualsearch/` package.
- No `ENABLE_VISUAL_SEARCH`, `VERTEX_EMBEDDING_*`, or `VECTOR_SEARCH_*` env vars.
- No Vector Search collection setup requirement.
- Future Google work should add Cloud Vision evidence extraction instead of vector DB search.
- Query quality remains `unknown` / `not_measured` until actual image-quality/OCR logic exists.

## Errors Encountered
| Error | Attempt | Resolution |
|-------|---------|------------|
| Existing plan drifted into Vector Search despite no-vector requirement | 1 | Rewrote plan and removed Vector Search/embedding implementation. |
| `gopls` not installed for Go LSP diagnostics | 1 | Use `go test`, `go build`, and `go vet` as fallback diagnostics. |

## Last Updated
2026-05-13T02:20:00+09:00


## Added Phase: Real Cloud Vision Evidence Integration
7. [in_progress] Add no-vector Cloud Vision evidence provider for Web Detection, OCR, Logo Detection, and Label Detection.
8. [pending] Feed Cloud Vision evidence into Bedrock recognition/search-query synthesis.
9. [pending] Expose evidence and measured status in API/UI.
10. [pending] Verify backend/frontend builds, tests, vet/typecheck, and implementation review.

## New Decisions
- Cloud Vision is an evidence layer, not vector search.
- Cloud Vision failures should not break the full search path unless configuration requires provider startup and startup itself fails.
- Runtime status must be explicit: disabled, measured, or error.


## Added Phase: Multi-Image SSE Extension Design
11. [complete] Investigate current single-image/crop flow and streaming gaps.
12. [complete] Design max-5 image request model, evidence fusion, and streaming endpoint.
13. [complete] Create `MULTI_IMAGE_SSE_DESIGN.md` for implementation planning.
14. [pending] Implement multi-image/SSE extension in a later phase.

## New Decisions - 2026-05-16
- Keep existing `POST /api/recognize-search` as backward-compatible final JSON endpoint.
- Add `POST /api/recognize-search-stream` using fetch + ReadableStream SSE framing because image JSON bodies do not fit native GET-only EventSource.
- Treat accuracy as coverage/agreement/confidence progression, not an unmeasured numeric accuracy percentage.
- Process up to five images into one integrated answer using fused Cloud Vision evidence and one integrated LLM recognition call where provider limits allow.

## Errors Encountered - 2026-05-16
| Error | Attempt | Resolution |
|---|---|---|
| JavaScript writer failed on markdown code fences in template literal | 1 | Rewrote file generation using Python raw string writer. |


## 2026-05-17 Implementation update

Status: local runnable implementation complete.

Completed phases:
1. Backend models/config/validation for v2 multi-image requests.
2. Backend SSE endpoint and shared usecase event emission.
3. Middleware compatibility for `http.Flusher`.
4. Frontend stream API client, hook, multi-image capture tray, event timeline, and final result display.
5. Local verification with backend tests/build/vet, frontend typecheck/build, and mock SSE progressive stream.

Open follow-up for production refinement:
- Extend real Bedrock prompt construction to send all images natively instead of primary-image normalization.
- Extend Cloud Vision extraction to run per-image worker-limited extraction when enabled.
- Run deployed Cloud Run progressive flush gate after deployment infrastructure is available.


## 2026-05-17 Final implementation status after review

Status: complete after blocker fixes.

Resolved review blockers:
- Secondary images now contribute to LLM request data and mock final result.
- Vision extraction now supports all images and merges evidence with image-id prefixes.
- SSE event emission is race-safe and verified with `go test -race`.
- Heartbeat goroutine stops before handler return.
- Base64 byte accounting and inputSummary mode semantics fixed.

All local verification gates passed. Remaining production-only work:
- Deploy-time Cloud Run progressive flush gate.
- Provider-specific optimization for native Bedrock multi-image content and worker-limited Cloud Vision concurrency beyond current sequential multi-image extraction.


## 2026-05-17 Static passphrase gate

Goal: add a lightweight passphrase gate for the GitHub Pages frontend before camera/search features load.

Phases:
1. [complete] Locate frontend entry points and config patterns.
2. [complete] Choose client-side PBKDF2-SHA256 hash verification with `sessionStorage` unlock state.
3. [complete] Add gate UI, auth hook, env example, hash generator, and README setup notes.
4. [complete] Verify frontend diagnostics, typecheck, and build.

Decision: this is a convenience gate only; GitHub Pages static bundles cannot provide real protection for secrets or confidential content.

Errors Encountered:
| Error | Attempt | Resolution |
|---|---|---|
| `ctx_execute` wrote from a temp cwd and failed to find `frontend/.env.example` | 1 | Switched to direct repo patch with absolute paths. |
| `crypto.subtle.deriveBits` rejected `Uint8Array<ArrayBufferLike>` salt type | 1 | Changed salt conversion to return `ArrayBuffer`. |

Verification:
- LSP diagnostics passed for `frontend/src/App.tsx`, `frontend/src/components/PassphraseGate.tsx`, `frontend/src/hooks/usePassphraseAuth.ts`, `frontend/src/config/auth.ts`, and `frontend/src/vite-env.d.ts`.
- `cd frontend && npm run auth:hash -- "sample-long-passphrase-for-check"` passed.
- `cd frontend && npm run typecheck` passed.
- `cd frontend && npm run build` passed.


## 2026-05-17 Deployment scripts and instructions

Goal: add deployment commands under each app's `bin/` directory and document environment-variable setup for GitHub Pages frontend and Cloud Run backend.

Known target:
- Frontend: GitHub Pages for `kakeru-ikeda/object-lens-search-demo`.
- Backend: Cloud Run in existing Google Cloud project `object-lens-search`.

Phases:
1. [complete] Inspect current deploy/build/server configuration and required env vars.
2. [complete] Confirm GitHub Pages and Cloud Run command details from official docs/references.
3. [complete] Add `frontend/bin/deploy` and `backend/bin/deploy`.
4. [complete] Document env setup and deployment order in README.
5. [complete] Verify shell syntax, frontend build/typecheck, backend test/build/vet, and docs consistency.

Decisions:
- Frontend deploy should build Vite with explicit GitHub Pages base path and publish only `frontend/dist` to `gh-pages`.
- Backend deploy should build from `backend/Dockerfile`, deploy to Cloud Run, and require production env/secret setup instead of reading local secret values into git.
- `ALLOWED_ORIGINS` for Cloud Run must include the final GitHub Pages origin.

Verification:
- `bash -n frontend/bin/deploy backend/bin/deploy` passed.
- deploy scripts are executable.
- `cd frontend && npm ci && npm run typecheck && npm run build` passed.
- `cd backend && go test ./... && go build ./cmd/server && go vet ./...` passed.
- Backend deploy missing-env guard stopped before deploy as expected.
- Frontend deploy missing-env/dirty-tree guard stopped before deploy as expected.

Known tool constraints:
- Frontend LSP reported missing React types before `npm ci`; package verification passed after install.
- Go LSP diagnostics unavailable because `gopls` is not installed; `go test/build/vet` passed as fallback.
- Final re-check passed: `bash -n` for both deploy scripts, executable bits, frontend `npm ci && npm run typecheck && npm run build`, frontend LSP diagnostics, and backend `go test ./... && go build ./cmd/server && go vet ./...`.
- Cloud Run Secret Manager IAM failure fixed by granting `roles/secretmanager.secretAccessor` to the default runtime service account and automating future grants in `backend/bin/deploy`.
- Backend Cloud Run deployment verified at `<cloud-run-service-url>/api/healthz`.
- Frontend GitHub Pages deployment verified at `https://kakeru-ikeda.github.io/object-lens-search-demo/`.
- Frontend deploy dirty-tree guard changed to opt-in (`REQUIRE_CLEAN_TREE=true`) because only `frontend/dist` is pushed to `gh-pages` via a temporary git repository.


## 2026-05-17 Progressive Parallel SSE Search

Goal: implement progressive search UX where the modal opens immediately, Cloud Vision evidence, Bedrock Haiku hypotheses, speculative Tavily searches, and final Bedrock conclusion stream through SSE as independently completed stages.

Phases:
1. [complete] Extend backend LLM/usecase contracts for Bedrock Haiku interim hypotheses and rich SSE payloads.
2. [complete] Add bounded speculative searches from approved query sources and dedupe aggregate results.
3. [complete] Extend frontend stream state and modal result panel for partial service cards before final data.
4. [complete] Add/update backend and frontend tests.
5. [complete] Verify Go tests/build/vet, frontend typecheck/build, streaming behavior, and implementation review.

Decisions:
- Lightweight interim LLM uses Bedrock model `global.anthropic.claude-haiku-4-5-20251001-v1:0` via `BEDROCK_LIGHT_MODEL_ID`.
- Interim LLM output is displayed as an unverified hypothesis, never as final answer.
- Speculative Tavily searches are bounded to approved query sources and final conclusion uses an adopted snapshot so late partial events cannot overwrite final.

Verification:
- `cd backend && go test ./... && go build ./cmd/server && go vet ./...` passed.
- `cd backend && go test -race ./internal/usecase ./internal/handler` passed.
- Frontend LSP diagnostics passed for modified TS/TSX files.
- `cd frontend && npm run typecheck && npm run build` passed.
- Local mock SSE returned progressive `llm_hypothesis_completed`, `query_generated`, `search_results`, and `final` events, with duplicate primary/speculative search-start removed.
