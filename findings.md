# Findings: Vector Search + Web Search Architecture

## Phase 1: Document Discovery & OSS Examples

### Local Image Vector Search (CLIP-based)

**Key OSS Projects (2026)**:

1. **pixmap** (altamsh04/pixmap)
   - On-device CLIP embeddings + HNSW indexing + SQLite
   - Stack: @xenova/transformers (CLIP ViT-B/32 via ONNX), sharp, hnswlib-node, better-sqlite3
   - 512-dim embeddings, ~2KB per image, 100K images ≈ 220MB
   - CLI: `pixmap add ./photo.jpg`, `pixmap search ./query.jpg -k 10`
   - No cloud, no API keys, images stay private

2. **local-image-search** (Eventual-Inc)
   - MLX CLIP (Apple Silicon optimized) + Lance vector DB + Daft batch processing
   - 260+ images/sec on M1/M2/M3/M4
   - MCP server for Claude integration
   - Natural language search: embed query → Lance retrieval
   - Cached embeddings in `embeddings.lance/`

3. **image-archive-search** (PyPI v0.1.0, 2026-04-08)
   - CLIP zero-shot enrichment + FAISS + SQLite + FastAPI
   - Incremental indexing, text + image-to-image search
   - Folder/date/content-type filtering
   - Structured enrichment: tags, styles, objects via CLIP zero-shot
   - Optional Ollama backend for richer VLM enrichment

4. **CLIP-database** (droon/CLIP-database)
   - SigLIP 2 embeddings (1152-dim) + sqlite-vec
   - Text search, image search, combined search with weighted blending
   - Interactive mode: load model once, run multiple queries
   - HTML gallery output with `localexplorer:` protocol links

5. **Zebra Frontline AI Product Recognition**
   - Feature Extractor → Feature Storage (vector DB) → Recognizer
   - FAISS index + SKU labels
   - Supports bounding box regions (for shelf/product localization)
   - Retail-optimized: shelf detection + product cropping + recognition

### Web Image Search Integration

**Tavily Search (2026 Update)**:
- `include_images=true` returns source-linked images on each result
- Per-result `images` array: diagrams, charts, screenshots, product visuals
- No separate image search needed; text + images in single response
- `include_image_descriptions=true` for AI-generated descriptions
- Works across all search depths (basic/advanced)

**Google Vision API Product Search**:
- Catalog → ProductSet → Index → Endpoint → BatchAnalyze
- Visual embedding + OCR text signals for recognition
- Supported categories: homegoods, apparel, toys, packaged goods, general
- GTIN/UPC-level product identity

**Google Vertex AI Product Recognizer**:
- Product visual embedding model
- OCR extraction
- Entity extraction (customizable key-value pairs)
- Recognizes products at GTIN/UPC level

### Barcode/OCR Augmentation

**Zebra Product Recognition**:
- Feature Extractor generates descriptors from images or bounding boxes
- Recognizer finds top-K matches from FAISS index
- SKU labels stored alongside embeddings
- Supports barcode/OCR preprocessing before recognition

**Google Vision OCR**:
- Extracts all visible text from images
- Combined with visual embeddings for product matching
- Entity extraction for structured data (brand, size, etc.)

### Semantic Product Search Architecture (2026)

**Hybrid Search Pattern** (XICTRON blog, 2026-04-30):
- Dense search (vector embeddings) + Sparse search (BM25)
- Reciprocal Rank Fusion (RRF): +15-30% recall over single methods
- Cross-encoder reranking on top-K candidates (20-80ms latency)
- Total latency: 50-200ms (embedding 10-50ms, HNSW 7-16ms, rerank 20-80ms)
- Handles synonyms, intent variants, long-tail queries
- Conversion uplift: 2-3x higher (Algolia), Amazon 2% → 12%

## Phase 2: Architecture Patterns (Emerging)

### Pattern 1: LLM Label → Web Search → Image Crawl → Local Index

```
User Image
  ↓
[Bedrock Vision] → "Coca-Cola Classic Can"
  ↓
[Tavily Search] → Web results + source-linked images
  ↓
[Image Crawl] → Top 5-10 product images from results
  ↓
[CLIP Embed] → 512-dim vectors
  ↓
[HNSW Index] → Local catalog (SQLite metadata)
  ↓
[Future Query] → "Red can with white label" → vector search
```

**Pros**:
- Live web retrieval for latest products
- Persistent local index for fast repeat queries
- Combines LLM reasoning + vector similarity

**Cons**:
- Crawling ToS concerns (see Phase 5)
- Rate limits on Tavily (1000 credits/month free)
- Image quality/relevance varies

### Pattern 2: Persistent Catalog + Live Web Fallback

```
User Image
  ↓
[CLIP Embed] → 512-dim vector
  ↓
[Local HNSW Search] → Top-K matches from indexed catalog
  ↓
If confidence < threshold:
  → [Tavily Search] → "Coca-Cola" + include_images
  → [Update Index] → Add new images to catalog
```

**Pros**:
- Fast local search first
- Web search only when needed
- Reduces API calls

**Cons**:
- Requires pre-populated catalog
- Stale products not discovered

### Pattern 3: Barcode/OCR → Product DB Lookup → Vector Refinement

```
User Image
  ↓
[Barcode Detection] → UPC/GTIN extracted
  ↓
[Product DB Lookup] → Exact match (Google Product DB or custom)
  ↓
If no match:
  → [CLIP Embed] → Vector search
  → [Tavily Search] → "UPC 049000050127" (Coca-Cola)
```

**Pros**:
- Deterministic for barcoded products
- Fallback to fuzzy matching

**Cons**:
- Requires barcode visibility
- Product DB maintenance

## Phase 3: Coca-Cola Can Demo Case

**Scenario**: User points camera at Coca-Cola Classic can.

**Flow**:
1. Frontend captures image, sends to backend
2. Bedrock Vision: "Coca-Cola Classic Can, red label, white text"
3. Tavily Search: `query="Coca-Cola Classic Can"`, `include_images=true`
   - Returns: Wikipedia, official Coca-Cola, retail sites + product images
4. Extract top 3-5 images from results
5. CLIP embed each image → 512-dim vectors
6. Store in local HNSW index with metadata:
   ```json
   {
     "id": "coca-cola-classic-1",
     "sku": "049000050127",
     "source_url": "https://...",
     "embedding": [0.123, -0.456, ...],
     "metadata": {
       "product_name": "Coca-Cola Classic",
       "size": "12 oz",
       "region": "US",
       "indexed_date": "2026-05-12"
     }
   }
   ```
7. Future queries: "red soda can" → embed → HNSW search → top-K results

**Barcode Augmentation**:
- If barcode detected: UPC 049000050127 → Coca-Cola Classic (deterministic)
- Confidence boost in vector search results

## Phase 4: Live Web Retrieval vs. Persistent Catalog

| Aspect | Live Web (Tavily) | Persistent Catalog (Local) |
|--------|-------------------|---------------------------|
| **Latency** | 500-2000ms | 10-50ms |
| **Freshness** | Real-time | Stale (last indexed) |
| **Cost** | API credits | Storage (~2KB/image) |
| **Privacy** | Queries sent to Tavily | 100% local |
| **Coverage** | All products on web | Only indexed products |
| **Use Case** | Discovery, new products | Repeat queries, known catalog |

**Hybrid Recommendation**:
- Index top 100-1000 products locally (Coca-Cola variants, competitors)
- Live web search for unknown products
- Update local index weekly from web results

## Phase 5: Legal & Rate-Limit Concerns

### Web Scraping / Image Crawling

**Caveats**:
1. **Terms of Service**: Most sites prohibit automated image crawling
   - Google Images ToS: "You may not use the Images for any purpose other than as part of the Google Images search results"
   - Tavily: Respects robots.txt, but crawling images from results may violate source site ToS
   
2. **Copyright**: Product images are copyrighted
   - Fair use: Limited use for product recognition/research
   - Commercial use: Requires licensing or permission
   - Recommendation: Use official product images from manufacturer APIs (Coca-Cola, PepsiCo, etc.)

3. **Rate Limits**:
   - Tavily: 1000 credits/month free (1 credit ≈ 1 search)
   - Google Vision API: Pay-per-request (~$0.10-0.50 per image)
   - CLIP embedding: Free (local), no rate limits

4. **Robots.txt Compliance**:
   - Respect `robots.txt` on source sites
   - Use `User-Agent` headers
   - Implement backoff/retry logic

### Recommended Approach

**For Coca-Cola Demo**:
1. Use official Coca-Cola product images (brand guidelines, press kit)
2. Tavily search for product info (text only, no image crawling)
3. Embed official images locally
4. Barcode lookup for deterministic matching

**For Production**:
1. Partner with retailers/manufacturers for product catalogs
2. Use official APIs (Google Product Search, Shopify, etc.)
3. Implement image licensing checks
4. Log all image sources for compliance

## Next Steps (Phase 3+)

- [ ] Implement CLIP embedding pipeline (pixmap or local-image-search)
- [ ] Integrate Tavily `include_images=true` for source-linked images
- [ ] Build HNSW index with SQLite metadata
- [ ] Test Coca-Cola can recognition with barcode + vector search
- [ ] Measure latency: embedding (10-50ms) + HNSW (7-16ms) + rerank (20-80ms)
- [ ] Document image source compliance

## References

- Pinecone Docs: https://docs.pinecone.io/guides/get-started/overview
- Tavily Docs: https://docs.tavily.com/documentation/api-reference/endpoint/search
- pixmap: https://github.com/altamsh04/pixmap
- local-image-search: https://github.com/Eventual-Inc/local-image-search
- image-archive-search: https://pypi.org/project/image-archive-search/
- XICTRON Semantic Product Search 2026: https://www.xictron.com/en/blog/semantic-product-search-vector-search-shops-2026/
- Google Vision Product Search: https://cloud.google.com/vision/product-search/docs
- Zebra Product Recognition: https://techdocs.zebra.com/ai-datacapture/2-22/productrecognition/

