# Findings: No-Vector Google Visual Search

## Current direction

The project now explicitly avoids customer-managed vector search, vector databases, embedding indexes, and Vector Search collections.

The chosen direction is:

```text
multi-crop image input
  ↓
Cloud Vision evidence extraction
  ├─ Web Detection
  ├─ OCR / text detection
  ├─ Logo Detection
  └─ Label Detection
  ↓
LLM evidence synthesis
  ↓
Tavily/web corroboration
  ↓
answer + ambiguity handling
```

## Why Vector Search was removed

- User clarified that vector search should not be required.
- Vector Search requires creating and maintaining a collection/index of product image vectors.
- That setup conflicts with the desired simpler Google/web-connected search approach.
- The app should not require `VECTOR_SEARCH_COLLECTION_ID`, `image_embedding`, or product image vector ingestion.

## What remains useful

### Cloud Vision Web Detection

Closest public Google Cloud feature to Lens-like web visual evidence without owning a vector DB.

Useful outputs:

- web entities.
- full matching images.
- partial matching images.
- visually similar images from public web evidence.
- best guess labels.

### OCR / Logo / Label Detection

Strong fit for packaged products such as Coca-Cola cans:

- OCR can find `Coca-Cola`, `Zero Sugar`, `Original Taste`, size, and region text.
- Logo Detection can confirm brand.
- Label Detection can supply coarse category like beverage/can/bottle.

### Product Recognizer

Optional later layer for GTIN/UPC or retail product hints. It may use Google-managed visual recognition internally, but does not require this app to own a vector collection.

### LLM + Tavily

Current MVP path remains valuable:

- LLM recognizes the image and creates a search query.
- Tavily retrieves web results.
- LLM summarizes and explains evidence.

## Tradeoffs without Vector Search

| Capability | Result without Vector Search |
|---|---|
| Private catalog image similarity | Not supported |
| Product DB management | Not required |
| Setup burden | Lower |
| Exact SKU visual matching | Weaker |
| Coca-Cola-style brand/package recognition | Still possible through OCR/logo/web entities |
| Lens-like web evidence | Possible via Web Detection |

## Implementation implications

Keep:

- multi-crop request support.
- `queryQuality` scaffold.
- Bedrock/Tavily MVP flow.
- future Cloud Vision provider seam.

Remove / avoid:

- `backend/internal/embedding/`.
- `backend/internal/visualsearch/`.
- `ENABLE_VISUAL_SEARCH`.
- `VERTEX_EMBEDDING_*`.
- `VECTOR_SEARCH_*`.
- product image vector collection setup.

## Next implementation step

Add a Cloud Vision evidence provider that calls `images:annotate` with:

- `WEB_DETECTION`
- `TEXT_DETECTION`
- `LOGO_DETECTION`
- `LABEL_DETECTION`

Then map those signals into an evidence object and feed it into the existing LLM/Tavily pipeline.


## 2026-05-13 Cloud Vision implementation findings

Official Go package: `cloud.google.com/go/vision/apiv1`. Use `vision.NewImageAnnotatorClient(ctx)` with ADC / `GOOGLE_APPLICATION_CREDENTIALS`. Batch request feature types: `WEB_DETECTION`, `TEXT_DETECTION`, `LOGO_DETECTION`, `LABEL_DETECTION`. Response fields: `WebDetection`, `TextAnnotations`, `LogoAnnotations`, `LabelAnnotations`.

Chosen implementation: add `backend/internal/vision` provider interface plus `cloudvision` and `mock` providers. Use tightCrop/imageBase64 as primary image for Cloud Vision, pass extracted evidence into LLM recognition prompt, and return it in `recognizedObject.visualEvidence` plus `queryQuality.evidenceTypes`.


## 2026-05-16 Multi-image SSE design findings

- Current frontend sends one crop set through `recognizeAndSearch` and waits for final JSON.
- Current backend validates one `imageBase64` or one `crops` payload, then runs Cloud Vision, Bedrock recognition, Tavily search, and Bedrock summary.
- Current Cloud Vision provider uses one primary image. Multi-image support should batch/merge evidence and keep per-image traceability.
- SSE should be implemented as POST + fetch ReadableStream, not browser EventSource, because the request body contains images.
- New streaming endpoint should preserve existing `POST /api/recognize-search` compatibility.


## 2026-05-16 Oracle design review findings

- The design direction is acceptable, but implementation would break unless final SSE payload remains a true `RecognizeSearchResponse` superset.
- `images[]` must be mutually exclusive with legacy `imageBase64/crops`.
- `options.maxImages` and `options.stream` must be added to Go/TypeScript types because the current handler rejects unknown fields.
- SSE payloads need a fixed envelope with top-level `elapsedMs` and monotonic `sequence`.
- Backend should use `ExecuteWithEvents` plus an `EventSink` so JSON and SSE paths share core logic.

## 2026-05-16 Final design decisions

- Multi-image streaming uses fixed limits: 2 MiB decoded per image, 10 MiB decoded total, 14 MiB HTTP body default, and 120s stream timeout.
- First implementation keeps Cloud Vision interface compatibility and runs per-image evidence extraction with worker limit 3.
- LLM uses one integrated multi-image call, with exactly one compact retry using primary image plus fused evidence if provider limits reject the full payload.
- UI must not show fake numeric accuracy; it shows evidence coverage and signal agreement.


## 2026-05-16 Final Oracle blocker fixes

- Usecase flow and event table now share the same fixed event names, including `summary_completed`.
- Frontend `StreamProgressEvent` includes top-level `elapsedMs` to match the SSE envelope.
- Cloud Run/proxy streaming is no longer a caveat: local and deployed progressive flush checks are mandatory release gates.


## 2026-05-17 Implementation findings

- Existing logging middleware wrapped `http.ResponseWriter` and hid `http.Flusher`; SSE handler returned `streaming_not_supported` until `statusRecorder.Flush()` was added.
- Local mock providers complete quickly enough to verify progressive stream behavior without external AWS/Tavily/GCP credentials.
- Initial backend implementation keeps existing provider interfaces stable by normalizing multi-image input to the selected primary image while exposing multi-image metadata and event progress. This keeps local runnable state and leaves deeper provider-level multi-image synthesis as a safe follow-up.
- `go test/build` updated `backend/go.mod` and `backend/go.sum` by promoting already-used Cloud Vision dependencies to direct requirements and adding a missing checksum.


## 2026-05-17 Review blocker fixes

- Goal review failed because secondary images did not affect service/data flow. Fixed by passing all `images[]` into `RecognizeObjectRequest.Images`, updating mock LLM output to reflect image count, and adding multi-image evidence extraction/merge across all images when Vision is enabled.
- Code quality review failed on concurrent `eventEmitter.sequence` access. Fixed by adding a mutex around sequence assignment and sink emission.
- Heartbeat lifecycle tightened: stream handler now cancels heartbeat and waits for goroutine exit before returning.
- Base64 size accounting now uses decoded byte length after successful decode instead of `DecodedLen` maximum.
- `inputSummary.mode` now returns `multi_image` for non-stream `images[]` and `multi_image_stream` only for stream requests.
- Added tests proving multi-image mock recognition changes final object and multi-image Vision evidence merges labels from all images.

Re-verification:
- `cd backend && go test ./...` passed.
- `cd backend && go test -race ./internal/usecase ./internal/handler` passed.
- `cd backend && go build ./cmd/server` passed.
- `cd backend && go vet ./...` passed.
- `cd frontend && npm run typecheck` passed.
- `cd frontend && npm run build` passed.
- Local mock SSE passed with 2 images: status 200, 10 events, first event <1s, >=2 events before final, final object `2枚のサンプル物体`, imageCount=2, responseVersion=2.
