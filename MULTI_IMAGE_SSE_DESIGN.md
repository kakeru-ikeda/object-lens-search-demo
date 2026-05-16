# Multi-Image SSE Extension Design

## Goal

Extend the demo from one captured image/crop set into up to five image inputs that are combined into one answer, while streaming staged progress to the frontend with Server-Sent Events (SSE). The UI shows the answer becoming more evidence-backed as more images and services contribute signals. It must not imply measured mathematical accuracy unless an actual metric is implemented.

## Current baseline

- Frontend captures one video frame region in `CameraView` and creates `tightCrop` plus `contextCrop` with `cropImageMultiVariant`.
- Frontend sends `POST /api/recognize-search` through `recognizeAndSearch` and waits for one final JSON response.
- Backend validates exactly one of `imageBase64` or `crops` in `RecognizeSearchHandler`.
- `RecognizeSearchUsecase.Execute` runs Cloud Vision evidence extraction in parallel with Bedrock image recognition, then Tavily search, then Bedrock summary.
- Cloud Vision currently picks one primary crop. Final response already has `visualEvidence`, `queryQuality`, and `stageLatency`, which are useful seams for progressive reporting.

## Product behavior

### Input model

Users can collect 1-5 image shots before running search.

Final UX specification:

1. Camera view remains primary.
2. Capture button adds current crop set to an input tray instead of immediately sending.
3. Tray shows thumbnails labeled `Image 1` ... `Image 5`.
4. User can remove/reorder images before search.
5. Search never starts automatically. The user explicitly presses `Search with N images`; this avoids accidental multi-megabyte streaming requests.

Use cases:

- Same object from multiple angles.
- Package front + ingredient/label side + barcode/serial closeup.
- Object plus context scene.
- Ambiguous object refined by extra shots.

### Accuracy storytelling

The frontend must not claim mathematical accuracy. It uses evidence coverage and agreement copy only:

- `input coverage`: how many images were accepted and processed.
- `evidence coverage`: OCR/logo/web/label evidence count by image.
- `agreement`: whether LLM recognition and Cloud Vision signals converge.
- `answer state`: draft -> evidence-backed -> web-corroborated -> summarized.

## API design

Keep `POST /api/recognize-search` for backward compatibility. Add a new streaming endpoint:

```text
POST /api/recognize-search-stream
Accept: text/event-stream
Content-Type: application/json
```

Use POST with `fetch()` + `ReadableStream`, not browser `EventSource`, because EventSource cannot send a JSON POST body without workarounds.

### Request v2

```json
{
  "images": [
    {
      "id": "img_1",
      "role": "primary",
      "crops": {
        "tightCrop": "data:image/jpeg;base64,...",
        "contextCrop": "data:image/jpeg;base64,...",
        "textEnhancedCrop": "data:image/webp;base64,..."
      }
    }
  ],
  "language": "ja",
  "options": {
    "maxSearchResults": 5,
    "maxImages": 5,
    "stream": true
  }
}
```

Backward-compatible aliases stay accepted:

- legacy `imageBase64` becomes one `images[0].imageBase64`.
- legacy `crops` becomes one `images[0].crops`.
- `images[]` and legacy `imageBase64/crops` are mutually exclusive. Requests containing `images` plus either legacy field must be rejected as ambiguous.

Validation:

- `images.length`: 1-5.
- each item must provide exactly one of `imageBase64` or `crops`.
- each crop keeps current MIME rules: jpeg/png/webp.
- decoded payload limits are fixed for the extension:
  - `MAX_IMAGE_BYTES`: 2 MiB per image input after base64 decode.
  - `MAX_TOTAL_IMAGE_BYTES`: 10 MiB across all images after base64 decode.
  - `MAX_REQUEST_BYTES`: raise default from 5 MiB to 14 MiB so JSON/base64 overhead fits five 2 MiB images.
- `REQUEST_TIMEOUT_SECONDS`: keep 30s for legacy JSON endpoint.
- `STREAM_REQUEST_TIMEOUT_SECONDS`: add 120s for `/api/recognize-search-stream`.
- reject unknown fields as today.
- add Go/TypeScript fields for `options.maxImages` and `options.stream`; otherwise current `DisallowUnknownFields()` rejects the v2 example.

### Response final shape

The final event must contain a real superset of current `RecognizeSearchResponse`. The existing `requestId`, `queryQuality`, `recognizedObject`, `ambiguity`, `search`, `summary`, and `meta` fields stay required; v2 adds multi-image fields:

```json
{
  "requestId": "req_...",
  "responseVersion": 2,
  "queryQuality": { "status": "measured", "cropConfidence": "received" },
  "recognizedObject": {
    "objectName": "...",
    "description": "integrated description",
    "searchQuery": "integrated query",
    "confidence": "high",
    "needsMoreContext": false,
    "visualEvidence": { "ocr": [], "logos": [], "webEntities": [] }
  },
  "ambiguity": { "isAmbiguous": false, "reason": "multi-image evidence agrees" },
  "search": {},
  "summary": {},
  "meta": {},
  "inputSummary": {
    "imageCount": 3,
    "processedImageCount": 3,
    "failedImageCount": 0
  },
  "imageAnalyses": [
    {
      "imageId": "img_1",
      "role": "primary",
      "queryQuality": { "status": "measured" },
      "visualEvidence": { "ocr": [], "logos": [], "webEntities": [] },
      "recognizedObject": { "objectName": "...", "confidence": "medium" }
    }
  ],
  "evidenceFusion": {
    "agreement": "strong",
    "supportingImageIds": ["img_1", "img_2"],
    "conflictingImageIds": [],
    "reason": "OCR/logo/web entities agree on brand and object category"
  }
}
```

## Backend design

### Data model additions

Add to `backend/internal/model/api.go`:

```go
type ImageInput struct {
    ID          string      `json:"id,omitempty"`
    Role        string      `json:"role,omitempty"`
    ImageBase64 string      `json:"imageBase64,omitempty"`
    Crops       *ImageCrops `json:"crops,omitempty"`
}

type RequestOptions struct {
    MaxSearchResults int  `json:"maxSearchResults,omitempty"`
    EnableMultiCrop  bool `json:"enableMultiCrop,omitempty"`
    MaxImages        int  `json:"maxImages,omitempty"`
    Stream           bool `json:"stream,omitempty"`
}

type RecognizeSearchRequest struct {
    ImageBase64 string         `json:"imageBase64,omitempty"`
    Crops       *ImageCrops    `json:"crops,omitempty"`
    Images      []ImageInput   `json:"images,omitempty"`
    Language    string         `json:"language,omitempty"`
    Options     RequestOptions `json:"options,omitempty"`
}
```

Provider-side model:

```go
type RecognizeImageInput struct {
    ID             string
    Role           string
    ImageDataURL   string
    Crops          *ImageCrops
    MIMEType       string
    CropMIMETypes  map[string]string
    VisualEvidence *VisualEvidence
}

type RecognizeObjectRequest struct {
    ImageDataURL   string
    Crops          *ImageCrops
    Images         []RecognizeImageInput
    Language       string
    VisualEvidence *VisualEvidence
}
```

### Usecase flow

Add an event-capable runner instead of forking business logic:

```go
type EventSink interface {
    Emit(ctx context.Context, event StreamEvent) error
}

func (u *RecognizeSearchUsecase) ExecuteWithEvents(ctx context.Context, req ExecuteRequest, sink EventSink) (*model.RecognizeSearchResponse, error)
```

`Execute` must call the same internal runner with a no-op sink so final JSON and SSE behavior cannot drift.

Final staged pipeline:

```text
normalize inputs
  ↓ emit request_received
for each image, run Cloud Vision evidence extraction concurrently with small worker limit
  ↓ emit image_evidence_started / image_evidence_completed per image
fuse visual evidence across images
  ↓ emit evidence_fused
run LLM integrated recognition once with all images + fused evidence
  ↓ emit recognition_started / recognition_completed
run Tavily using integrated query
  ↓ emit search_started / search_completed
run LLM summary with final object + results + evidence fusion
  ↓ emit summary_started / summary_completed
emit final
```

Decision: one integrated LLM recognition call is the default and required path:

- The target product is one combined answer, not five independent answers.
- Cross-image reasoning matters: front label + side text + context image.
- Cost/latency are lower than five independent LLM calls plus another fusion call.

Provider limit handling:

- If Bedrock rejects the multi-image payload because of model/payload limits, retry once with only the primary image plus fused evidence from all images.
- The retry must emit `recognition_retry_compacted` before retrying.
- If the compact retry fails, emit fatal `error` and keep the last draft UI state.
- Do not add a separate `RecognizeImages` method in the first implementation; extend `RecognizeObjectRequest.Images` and keep provider changes localized.

### Cloud Vision multi-image

Cloud Vision implementation is fixed for the first version:

- Keep the current `ExtractEvidence(ctx, ExtractEvidenceRequest)` interface.
- Call it per image with worker limit `min(3, imageCount)` to avoid a large provider/interface refactor in the same change.
- Preserve per-image status even when one image fails.
- A future optimization may add `ExtractEvidenceBatch`, but it is explicitly out of scope for the first implementation.

Normalize response per image:

```go
type ImageEvidence struct {
    ImageID   string
    Evidence  VisualEvidence
    Status    string
    ElapsedMs int64
}
```

Fused evidence rules:

- Deduplicate by lowercase trimmed text/url.
- Keep source image IDs for traceability.
- Prefer OCR/logo terms that appear in more than one image.
- Cap final fused evidence to prompt-safe limits.
- Do not merge conflicting strings silently; add `evidenceFusion.conflicts`.

## SSE event design

### Envelope

The SSE `event:` name and JSON payload must be fixed. `elapsedMs` belongs at the top level for stage timing; service-specific timings may also appear under `payload`. Heartbeats may be SSE comments and are the only frame without JSON payload.

```json
{
  "requestId": "req_123",
  "sequence": 5,
  "stage": "vision",
  "status": "completed",
  "elapsedMs": 420,
  "imageId": "img_2",
  "message": "Image 2: OCR and logo evidence found",
  "payload": {
    "evidenceTypes": ["ocr", "logo"],
    "topSignals": ["Coca-Cola", "Zero Sugar"]
  }
}
```

### Transport

Headers:

```text
Content-Type: text/event-stream
Cache-Control: no-cache, no-transform
Connection: keep-alive
X-Accel-Buffering: no
```

Each event:

```text
event: image_evidence_completed
data: {"requestId":"req_...","sequence":4,"stage":"vision","status":"completed","elapsedMs":420,"imageId":"img_2","message":"Image 2: OCR and logo evidence found","payload":{...}}

```

Always include:

- `requestId`
- `sequence`
- `stage`
- `status`
- `elapsedMs` at the top level for non-heartbeat events
- `message` for UI copy

### Event types

| Event | Purpose | UI effect |
|---|---|---|
| `request_received` | accepted N images | show input tray locked |
| `image_queued` | each image registered | create per-image row |
| `image_evidence_started` | Cloud Vision begins | service chip spins |
| `image_evidence_completed` | OCR/logo/web/label found | reveal evidence chips |
| `image_evidence_error` | one image failed Vision | mark image warning, continue |
| `evidence_fused` | cross-image evidence merged | show agreement/conflict meter |
| `recognition_started` | LLM sees images/evidence | show draft-answer area |
| `recognition_retry_compacted` | multi-image LLM payload was too large/unsupported, retrying primary image + fused evidence | show non-fatal compact retry note |
| `recognition_completed` | integrated object/query produced | update main result draft |
| `search_started` | Tavily starts | show query used |
| `search_completed` | web results returned | show result count/source list |
| `summary_started` | final summary starts | show refining state |
| `summary_completed` | final summary text is ready | show final summary preview before final payload |
| `final` | complete response | replace draft with final response |
| `error` | fatal error | show recoverable message |
| `heartbeat` | keep connection alive | no visible UI |

### Example event payloads

```json
{
  "requestId": "req_123",
  "sequence": 5,
  "stage": "vision",
  "status": "completed",
  "elapsedMs": 420,
  "imageId": "img_2",
  "message": "Image 2: OCR and logo evidence found",
  "payload": {
    "evidenceTypes": ["ocr", "logo"],
    "topSignals": ["Coca-Cola", "Zero Sugar"]
  }
}
```

```json
{
  "requestId": "req_123",
  "sequence": 8,
  "stage": "fusion",
  "status": "completed",
  "elapsedMs": 780,
  "message": "3 images agree on brand and package type",
  "payload": {
    "agreement": "strong",
    "supportingImageIds": ["img_1", "img_2", "img_3"],
    "conflictingImageIds": [],
    "coverageChange": "more_evidence_backed"
  }
}
```

## Frontend design

### State model

Add types in `frontend/src/types/index.ts`:

```ts
export interface ImageInputDraft {
  id: string;
  role: 'primary' | 'detail' | 'context' | 'label' | 'other';
  crops?: ImageCrops;
  thumbnail: string;
  status: 'queued' | 'processing' | 'done' | 'warning' | 'error';
  evidenceTypes?: string[];
  topSignals?: string[];
}

export interface StreamProgressEvent {
  requestId: string;
  sequence: number;
  stage: 'input' | 'vision' | 'fusion' | 'recognition' | 'search' | 'summary' | 'final';
  status: 'queued' | 'started' | 'completed' | 'warning' | 'error';
  elapsedMs: number;
  imageId?: string;
  message: string;
  payload?: Record<string, unknown>;
}
```

Hook plan:

- Keep `useRecognizeSearch` for non-stream fallback.
- Add `useRecognizeSearchStream` with:
  - `images`
  - `events`
  - `draftResult`
  - `finalData`
  - `activeStage`
  - `startStream(images)`
  - `abort()`

API client addition:

```ts
export async function recognizeAndSearchStream(
  req: RecognizeSearchRequest,
  handlers: {
    onEvent(event: StreamProgressEvent): void;
    onFinal(data: RecognizeSearchResponse): void;
    onError(error: Error): void;
  },
  signal?: AbortSignal,
): Promise<void>
```

Use `fetch` streaming parser:

- POST JSON body.
- Read chunks from `response.body.getReader()`.
- Parse SSE frames separated by blank lines, handle multi-line `data:` fields, comment heartbeats, and a trailing partial frame.
- Support `AbortController`.

### UI components

Add components:

- `ImageTray`: thumbnails, remove/reorder, max 5 indicator. First image is primary; reorder changes primary image and prompt order.
- `StreamTimeline`: service stages and messages.
- `EvidenceChips`: OCR/logo/web/label chips by image.
- `ConfidenceEvolution`: copy-based progression, not fake numeric certainty.
- `DraftResultPanel`: partial object/query/search status before final result.

Current `CameraView` remains owner of camera and capture. It must switch from immediate `fetchSearch(crops)` to `addImage(crops)`, then `startStream`.

## Failure handling

- One image Vision failure emits `image_evidence_error` and continues if at least one image remains valid.
- If all image evidence extraction fails, continue to LLM with raw images and emit `evidence_fused` with `agreement: "unknown"`.
- LLM failure after compact retry is fatal because integrated recognition cannot complete.
- Search failure emits fatal `error`, but the frontend keeps `recognition_completed` as a partial non-final draft and labels it `Web search failed`.
- Client abort cancels backend context and emits no final event.
- Heartbeat is emitted every 10 seconds as an SSE comment frame.
- Go handler must assert `http.Flusher`; if unavailable, return `500 streaming_not_supported` before starting work.
- Go handler must flush after every event and every heartbeat.
- Request timeout must use `STREAM_REQUEST_TIMEOUT_SECONDS` and cover the full stream duration; client abort must cancel backend context.
- CORS must allow the streaming endpoint and `Accept: text/event-stream`.
- Progressive flush verification is a release gate: in local and deployed Cloud Run environments, a mock-provider stream must show `request_received` in the browser within 1 second and at least two later stage events before `final`. If events arrive only at the end, deployment is blocked until buffering is removed from the serving path.
- `X-Accel-Buffering: no` is still sent for nginx-like proxies, but it is not treated as sufficient by itself; the release gate above is mandatory.
- `Connection: keep-alive` is HTTP/1.1-specific and may be ignored/invalid under HTTP/2; heartbeat + flush verification are the required keepalive/progress mechanisms.
- Sequence numbers must be monotonic; client must ignore older sequence values for the same request.

## Rollout plan

1. Add config limits: `MAX_IMAGE_BYTES=2097152`, `MAX_TOTAL_IMAGE_BYTES=10485760`, `MAX_REQUEST_BYTES=14680064`, `STREAM_REQUEST_TIMEOUT_SECONDS=120`.
2. Add data model and validation for `images[]` while preserving legacy request.
3. Add backend stream writer and `/api/recognize-search-stream` route with `http.Flusher`, heartbeat, and abort handling.
4. Refactor usecase into event-capable internal runner so JSON and SSE endpoints share core logic.
5. Add per-image Cloud Vision worker-limit path and fused evidence merge.
6. Add multi-image Bedrock prompt/content construction with compact retry to primary image + fused evidence.
7. Add frontend `recognizeAndSearchStream`, SSE parser, AbortController support, image tray, and stream hook.
8. Add timeline/draft UI and final result compatibility.
9. Verify with backend tests, frontend typecheck/build, and mandatory local + deployed mock-provider progressive flush checks.

## Final implementation decisions

- Image roles are user-selectable from `primary`, `detail`, `context`, `label`, and `other`; default is `primary` for the first image and `detail` for later images.
- Reordering changes prompt order. The first image is always the primary image.
- Size limits are fixed: 2 MiB decoded per image, 10 MiB decoded total, 14 MiB HTTP request body default.
- Final stream payload remains a backward-compatible `RecognizeSearchResponse` superset and adds `responseVersion: 2`.
- Native browser `EventSource` is not used for this feature; POST streaming uses `fetch` + `ReadableStream`.
- First implementation keeps Cloud Vision interface compatibility with per-image worker-limited calls, not batch interface refactor.
- UI wording says `evidence-backed`, `coverage increased`, and `signals agree`; it never displays fake numeric accuracy.
