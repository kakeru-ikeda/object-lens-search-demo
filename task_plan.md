# Task Plan: Object Lens Search MVP Implementation

## Goal
Implement the DESIGN.md backend MVP: Go API with Bedrock + Tavily provider boundaries.

## Scope
- Backend /healthz and /api/recognize-search with validation, CORS, rate limit, request ID, JSON logging.
- Provider interfaces: VisionLLM and WebSearcher.
- Mock fallback, Bedrock adapter, Tavily adapter, normalized models.
- Backend .env.example and Dockerfile.

## Phases
1. [complete] Restore planning context and read DESIGN.md backend requirements.
2. [complete] Scaffold backend Go module structure and config.
3. [complete] Implement backend models, middleware, handlers, usecase.
4. [complete] Implement mock, Bedrock, and Tavily providers.
5. [complete] Add backend env example, Dockerfile, and tests.
6. [complete] Verify with gofmt, diagnostics, go test, and go build.
6. [pending] Verify with gofmt, diagnostics, go test, and go build.

## Decisions
- Backend: Go net/http to avoid unnecessary router dependencies.
- MVP providers: Bedrock + Tavily fixed by env; mock mode allowed only when APP_ENV != production and credentials/config are missing, or provider env explicitly mock.
- Do not touch frontend/.

## Errors Encountered
| Error | Attempt | Resolution |
|-------|---------|------------|
| gopls not installed for lsp_diagnostics | 1 | Used go vet ./... as Go diagnostics fallback; go test/build passed. |

## Last Updated
2026-05-12T12:30:00.000Z

7. [complete] Fix review blockers.

## Review Fixes
- Fixed late getUserMedia stream cleanup in frontend camera hook.
- Fixed object-cover crop coordinate mapping.
- Fixed rate limiter IP normalization and expired bucket pruning.
- Replaced backend internal error details with public messages.
- Improved frontend API error parsing and close UX.

## Last Updated
2026-05-12T12:08:21.564876+00:00
