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
