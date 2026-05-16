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
./bin/dev
```

Set `VITE_API_BASE_URL` when the backend is not running at `http://localhost:8080`.

### Frontend passphrase gate for GitHub Pages

The frontend has a static-site passphrase gate before the camera/search UI. Generate a PBKDF2-SHA256 hash from the shared passphrase and set the values during the GitHub Pages build:

```bash
cd frontend
npm run auth:hash -- "choose-a-long-shared-passphrase"
```

Copy the output into `frontend/.env` for local testing, or into GitHub Actions / Pages build variables:

```bash
VITE_AUTH_PASSPHRASE_HASH=...
VITE_AUTH_PASSPHRASE_SALT=...
VITE_AUTH_PASSPHRASE_ITERATIONS=600000
```

Authenticated state is stored in `sessionStorage`, so a browser tab stays unlocked across reloads and locks again after the tab is closed. This is only a lightweight access gate for static hosting: all JavaScript, hashes, and public assets shipped by GitHub Pages remain inspectable in the browser. Do not use this to protect personal data, API secrets, paid content, or confidential documents; use server-side authentication for real security.

## Smartphone as PC webcam testing

Use this when you want the phone camera to appear as a PC webcam in the desktop browser.

Requirements for HTTP/MJPEG mode:

- A phone webcam app that exposes an MJPEG/HTTP video URL, for example `http://PHONE_IP:8080/video`.
- `v4l2loopback` on the PC.
- `ffmpeg` on the PC.

Create a virtual webcam device:

```bash
sudo modprobe v4l2loopback devices=1 video_nr=10 card_label="Phone Camera" exclusive_caps=1
```

Start Vite and pipe the phone stream into `/dev/video10`:

```bash
cd frontend
PHONE_CAM_URL="http://PHONE_IP:8080/video" ./bin/dev-phone
```

USB Android alternative with `scrcpy`:

```bash
sudo modprobe v4l2loopback devices=1 video_nr=10 card_label="Phone Camera" exclusive_caps=1
cd frontend
PHONE_CAM_MODE=scrcpy ./bin/dev-phone
```

`PHONE_CAM_MODE=auto` chooses HTTP mode when `PHONE_CAM_URL` is set, otherwise tries `scrcpy` if an adb device is connected.

Optional overrides:

```bash
PHONE_CAM_MODE=auto|http|scrcpy
VIDEO_DEVICE=/dev/video10
VIDEO_SIZE=1280x720
VIDEO_FPS=30
```

In the desktop browser camera picker, choose `Phone Camera`.


## Local backend

```bash
cd backend
cp .env.example .env
go test ./...
./bin/dev
```

Without production credentials, local development can use mock fallback outside `APP_ENV=production`. Production must set Bedrock and Tavily environment variables.

## Deployment

Deploy the backend first, then use the printed Cloud Run URL for the frontend build.

### 1. Cloud Run backend

Prerequisites:

- `gcloud` is installed and logged in.
- Google Cloud project `object-lens-search` exists.
- Billing is enabled for the project.
- AWS Bedrock credentials and Tavily API key are available outside git.

Create Secret Manager entries for the sensitive values. Do not commit these values to `.env` files:

```bash
gcloud config set project object-lens-search
gcloud services enable secretmanager.googleapis.com

printf '%s' 'YOUR_TAVILY_API_KEY' | gcloud secrets create tavily-api-key --data-file=-
printf '%s' 'YOUR_AWS_ACCESS_KEY_ID' | gcloud secrets create aws-access-key-id --data-file=-
printf '%s' 'YOUR_AWS_SECRET_ACCESS_KEY' | gcloud secrets create aws-secret-access-key --data-file=-
```

The deploy script grants `roles/secretmanager.secretAccessor` to the Cloud Run runtime service account for the configured secrets. If you deploy with a custom Cloud Run service account, set `SERVICE_ACCOUNT` in `backend/.env.deploy`; otherwise the script uses the default Compute Engine service account shown in Cloud Run errors as `PROJECT_NUMBER-compute@developer.gserviceaccount.com`.

If you need to grant access manually after a failed deploy, run:

```bash
RUNTIME_SERVICE_ACCOUNT='1054055053285-compute@developer.gserviceaccount.com'
gcloud secrets add-iam-policy-binding tavily-api-key \
  --member="serviceAccount:${RUNTIME_SERVICE_ACCOUNT}" \
  --role='roles/secretmanager.secretAccessor'
gcloud secrets add-iam-policy-binding aws-access-key-id \
  --member="serviceAccount:${RUNTIME_SERVICE_ACCOUNT}" \
  --role='roles/secretmanager.secretAccessor'
gcloud secrets add-iam-policy-binding aws-secret-access-key \
  --member="serviceAccount:${RUNTIME_SERVICE_ACCOUNT}" \
  --role='roles/secretmanager.secretAccessor'
```

Prepare deploy config:

```bash
cp backend/.env.deploy.example backend/.env.deploy
```

Edit `backend/.env.deploy`:

```bash
PROJECT_ID=object-lens-search
REGION=asia-northeast1
SERVICE_NAME=object-lens-search-api
ALLOWED_ORIGINS=https://kakeru-ikeda.github.io
APP_ENV=production
AWS_REGION=us-east-1
BEDROCK_MODEL_ID=anthropic.claude-3-5-sonnet-20241022-v1:0
CLOUD_VISION_ENABLED=false
# Optional custom runtime identity:
# SERVICE_ACCOUNT=object-lens-runner@object-lens-search.iam.gserviceaccount.com
GRANT_SECRET_ACCESS=true
```

Deploy:

```bash
cd backend
./bin/deploy
```

The script runs Go tests/build/vet, enables required Google Cloud APIs, creates an Artifact Registry repository if missing, builds the Docker image with Cloud Build, deploys Cloud Run, and prints the service URL.

Useful checks after deployment:

```bash
curl "$(gcloud run services describe object-lens-search-api --region asia-northeast1 --format='value(status.url)')/healthz"
curl "$(gcloud run services describe object-lens-search-api --region asia-northeast1 --format='value(status.url)')/api/healthz"
gcloud run services logs read object-lens-search-api --region asia-northeast1 --limit 50
```

If `CLOUD_VISION_ENABLED=true`, grant the Cloud Run runtime service account permission to call Cloud Vision. Do not set `GOOGLE_APPLICATION_CREDENTIALS` on Cloud Run; use the service account attached to the service.

### 2. GitHub Pages frontend

Generate the static passphrase hash:

```bash
cd frontend
npm run auth:hash -- "choose-a-long-shared-passphrase"
```

Prepare deploy config:

```bash
cp frontend/.env.deploy.example frontend/.env.deploy
```

Edit `frontend/.env.deploy` with the Cloud Run URL printed by backend deploy and the generated auth values:

```bash
GITHUB_REPOSITORY=kakeru-ikeda/object-lens-search-demo
PAGES_BASE_PATH=/object-lens-search-demo/
REQUIRE_CLEAN_TREE=false
VITE_API_BASE_URL=https://YOUR-CLOUD-RUN-SERVICE.a.run.app
VITE_AUTH_PASSPHRASE_HASH=...
VITE_AUTH_PASSPHRASE_SALT=...
VITE_AUTH_PASSPHRASE_ITERATIONS=600000
```

Deploy:

```bash
cd frontend
./bin/deploy
```

The script runs `npm ci`, `npm run typecheck`, builds Vite with the GitHub Pages base path, then publishes `frontend/dist` to the `gh-pages` branch through a temporary git repository. It does not require a clean source working tree by default because only the generated `dist` files are pushed to `gh-pages`; set `REQUIRE_CLEAN_TREE=true` if you want that stricter local policy. In GitHub repository settings, configure Pages to publish from the `gh-pages` branch.

For project pages, the site URL is:

```text
https://kakeru-ikeda.github.io/object-lens-search-demo/
```

### Deployment environment files

- `backend/.env.deploy` and `frontend/.env.deploy` are ignored by git.
- `backend/.env.deploy.example` and `frontend/.env.deploy.example` are safe templates and should be committed.
- Vite `VITE_*` values are public in the built frontend bundle; never put API secrets there.

## Verification

```bash
cd frontend && npm run typecheck && npm run build && npm audit --audit-level=moderate
cd backend && go test ./... && go build ./cmd/server && go vet ./...
```
