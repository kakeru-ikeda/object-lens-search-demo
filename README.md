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

## Smartphone camera testing

Camera access requires a secure context. `http://localhost:5173` works on the PC because localhost is treated as secure, but `http://<PC-LAN-IP>:5173` from a smartphone is not secure, so the browser will not show a camera permission prompt.

Recommended quick tunnel:

```bash
cd frontend
npm run dev -- --host 127.0.0.1
cloudflared tunnel --url http://127.0.0.1:5173
```

Open the generated `https://*.trycloudflare.com` URL on the smartphone.

If the backend runs on the PC, expose it too or set the frontend env before starting Vite:

```bash
VITE_API_BASE_URL=https://<backend-tunnel-url> npm run dev -- --host 127.0.0.1
```

Alternative: use `ngrok http 5173`, or use `mkcert` with a trusted certificate installed on the smartphone.


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
