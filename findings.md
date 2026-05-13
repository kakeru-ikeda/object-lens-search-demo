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
