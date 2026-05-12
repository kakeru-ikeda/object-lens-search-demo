# Object Lens Search Demo

Camera-based object search MVP. Frontend captures the object inside the overlay, backend recognizes it with Bedrock, searches with Tavily, normalizes results, and returns a short summary.

## Stack
- Frontend: React + Vite + TypeScript 7.0 beta via `@typescript/native-preview@beta` and `tsgo`
- Backend: Go + net/http
- LLM: Amazon Bedrock
- Search: Tavily

## Local frontend

```bash
cd frontend
npm install
npm run typecheck
npm run dev
```

Set `VITE_API_BASE_URL` when the backend is not running at `http://localhost:8080`.

## Local backend

```bash
cd backend
cp .env.example .env
go test ./...
go run ./cmd/server
```

Without production credentials, local development can use mock fallback outside `APP_ENV=production`. Production must set Bedrock and Tavily environment variables.

## Verification

```bash
cd frontend && npm run typecheck && npm run build && npm audit --audit-level=moderate
cd backend && go test ./... && go build ./cmd/server && go vet ./...
```
