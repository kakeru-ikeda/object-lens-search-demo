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

## Verification

```bash
cd frontend && npm run typecheck && npm run build && npm audit --audit-level=moderate
cd backend && go test ./... && go build ./cmd/server && go vet ./...
```
