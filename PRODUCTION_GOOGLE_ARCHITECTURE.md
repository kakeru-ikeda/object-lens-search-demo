# No-Vector Google Visual Search Architecture

## 1. Purpose

This document defines the Google-based visual-search design for Object Lens Search **without customer-managed vector search, vector databases, or product image embedding indexes**.

Google Lens itself is not exposed as a public Google Cloud API. This design recreates a Lens-like experience with public APIs that do not require maintaining a vector collection.

## 2. Core constraint

Do not require:

- Vertex AI Vector Search.
- Vector Search collections.
- self-managed product image embeddings.
- `image_embedding` schemas.
- vector database operations.

The system may still use Google-managed visual signals internally, such as Cloud Vision Web Detection or Product Recognizer, but the application must not own a vector index.

## 3. Pipeline

```text
Camera frame
  ↓
multi-crop input
  ├─ tight_crop
  ├─ context_crop
  └─ optional text_enhanced_crop
  ↓
Google evidence extraction
  ├─ Cloud Vision Web Detection
  ├─ Cloud Vision OCR
  ├─ Cloud Vision Logo Detection
  ├─ Cloud Vision Label Detection
  └─ optional Product Recognizer / barcode hints
  ↓
candidate text/query synthesis
  ├─ brand names
  ├─ OCR tokens
  ├─ web entities
  ├─ best guess labels
  └─ LLM visual description
  ↓
Tavily / web search
  ↓
LLM evidence synthesis
  ↓
answer with confidence, ambiguity, and evidence
```

## 4. What this can and cannot do

### Can do

- Identify strong brands/logos such as Coca-Cola from OCR/logo/web entities.
- Use public web evidence from Cloud Vision Web Detection.
- Explain why a candidate was selected.
- Avoid managing product image embeddings or Vector Search resources.
- Keep the current Bedrock + Tavily MVP flow as fallback.

### Cannot do

- Directly compare the crop against a private catalog image embedding index.
- Guarantee exact SKU-level visual similarity between near-identical packages.
- Replace Google Lens one-to-one.

## 5. Google services

### 5.1 Cloud Vision Web Detection

Primary no-vector image-search signal.

Use it for:

- web entities.
- full matching images.
- partial matching images.
- visually similar images from the web.
- best guess labels.

This is the closest public Google Cloud feature to web-connected visual search without owning a vector index.

### 5.2 Cloud Vision OCR

Use OCR for product names, labels, variant text, and package markings.

For Coca-Cola can cases, OCR tokens such as `Coca-Cola`, `Zero Sugar`, `Original Taste`, `350ml`, and regional text are high-value evidence.

### 5.3 Cloud Vision Logo Detection

Use logo detection for brand confirmation.

Logo evidence should increase confidence only when consistent with OCR and web entities.

### 5.4 Cloud Vision Label Detection

Use label detection for coarse object/category signals, such as can, bottle, beverage, snack, shoe, or electronics.

Do not treat label detection as product identity.

### 5.5 Product Recognizer / barcode hints

Optional later layer.

Use only if the target use case benefits from GTIN/UPC or retail product hints. Product Recognizer may use Google-managed embeddings internally, but it does not require this app to create a vector collection.

## 6. Candidate synthesis

Generate candidate queries from evidence:

```json
{
  "ocr": ["Coca-Cola", "Original Taste"],
  "logos": ["Coca-Cola"],
  "webEntities": ["Coca-Cola", "cola", "soft drink"],
  "bestGuessLabels": ["coca cola can"],
  "labels": ["tin can", "soft drink", "beverage"]
}
```

Candidate query examples:

- `Coca-Cola Original Taste can`
- `Coca-Cola 350ml can Japan`
- `Coca-Cola red can white logo`

Then use Tavily/web search and LLM synthesis to choose a likely answer.

## 7. Confidence and ambiguity

High confidence requires multiple agreeing signals:

- OCR contains product/brand text.
- Logo detection agrees with OCR.
- Web Detection entities agree with logo/OCR.
- LLM visual description does not contradict evidence.

Return ambiguity when:

- OCR and logo disagree.
- Web Detection returns generic entities only.
- similar variants exist, such as Classic vs Zero vs Diet.
- no brand-specific evidence is found.

## 8. Response direction

Keep response grounded in evidence rather than vector similarity.

Recommended future response additions:

```json
{
  "visualEvidence": {
    "ocr": ["Coca-Cola"],
    "logos": ["Coca-Cola"],
    "webEntities": ["Coca-Cola"],
    "bestGuessLabels": ["coca cola can"],
    "matchingImageUrls": []
  },
  "ambiguity": {
    "isAmbiguous": false,
    "reason": "OCR, logo, and web entities agree"
  }
}
```

## 9. Existing repo mapping

Keep:

- `frontend/src/lib/cropImage.ts`: multi-crop input.
- `frontend/src/hooks/useRecognizeSearch.ts`: request flow.
- `backend/internal/model/api.go`: multi-crop request and query quality fields.
- `backend/internal/usecase/recognize_search.go`: LLM recognition + Tavily search orchestration.
- `backend/internal/llm/`: visual reasoning provider boundary.
- `backend/internal/search/`: web search provider boundary.

Remove or avoid:

- `backend/internal/embedding/`.
- `backend/internal/visualsearch/`.
- `VECTOR_SEARCH_*` env vars.
- Vertex embedding env vars.
- Vector Search collection setup.

Suggested future packages:

```text
backend/internal/vision/cloudvision/
backend/internal/vision/evidence/
backend/internal/productrecognizer/google/
backend/internal/pipeline/evidencesearch/
```

## 10. Implementation order

1. Keep multi-crop request support.
2. Add Cloud Vision Web Detection provider.
3. Add Cloud Vision OCR/logo/label provider.
4. Add evidence synthesis model types.
5. Use LLM to turn evidence into candidate queries.
6. Keep Tavily search for web corroboration.
7. Add ambiguity rules based on evidence agreement.
8. Add Product Recognizer only if GTIN/product hints are needed.

## 11. References

- Cloud Vision Web Detection: `https://cloud.google.com/vision/docs/detecting-web`
- Cloud Vision OCR / Logo / Label features: `https://cloud.google.com/vision/docs/features-list`
- Cloud Vision Label Detection: `https://cloud.google.com/vision/docs/labels`
- Product Recognizer: `https://cloud.google.com/vision-ai/docs/product-recognizer`
