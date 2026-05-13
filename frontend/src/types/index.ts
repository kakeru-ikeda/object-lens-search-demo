export interface EvidenceItem {
  text: string;
  score?: number;
}

export interface VisualEvidence {
  ocr?: EvidenceItem[];
  logos?: EvidenceItem[];
  webEntities?: EvidenceItem[];
  bestGuessLabels?: string[];
  labels?: EvidenceItem[];
  matchingImageUrls?: string[];
}

export interface RecognizedObject {
  objectName: string;
  description: string;
  searchQuery: string;
  confidence: 'low' | 'medium' | 'high';
  needsMoreContext: boolean;
  visualEvidence?: VisualEvidence;
}

export interface NormalizedSearchResult {
  id: string;
  title: string;
  url: string;
  displayUrl: string;
  snippet: string;
  source: string;
  publishedAt: string | null;
  language: string;
  rank: number;
  score: number;
  contentType: string;
  provider: string;
  raw: Record<string, unknown>;
}

export interface SearchSummary {
  text: string;
  llmProvider: string;
  model: string;
}

export interface ImageCrops {
  tightCrop: string;
  contextCrop: string;
  textEnhancedCrop?: string;
}

export interface QueryQuality {
  blur: 'low' | 'medium' | 'high' | 'unknown';
  cropConfidence: 'low' | 'medium' | 'high' | 'unknown' | 'received';
  textVisibility: 'low' | 'medium' | 'high' | 'unknown';
  status: 'not_measured' | 'multi_crop_received_not_measured' | 'multi_crop_received_cloud_vision_disabled' | 'cloud_vision_disabled' | 'cloud_vision_error' | 'cloud_vision_no_evidence' | 'measured' | string;
  evidenceTypes?: string[];
}

export interface Ambiguity {
  isAmbiguous: boolean;
  reason: string;
}

export interface RecognizeSearchResponse {
  requestId: string;
  queryQuality: QueryQuality;
  recognizedObject: RecognizedObject;
  ambiguity: Ambiguity;
  search: {
    provider: string;
    query: string;
    results: NormalizedSearchResult[];
  };
  summary: SearchSummary;
  meta: {
    llmProvider: string;
    searchProvider: string;
    cloudVisionProvider: string;
    elapsedMs: number;
    stageLatency: {
      cloudVisionMs: number;
      recognizeMs: number;
      searchMs: number;
      summarizeMs: number;
    };
  };
}

export interface RecognizeSearchRequest {
  imageBase64?: string;
  crops?: ImageCrops;
  language?: string;
  options?: {
    maxSearchResults?: number;
    enableMultiCrop?: boolean;
  };
}

export interface ErrorResponse {
  error: {
    code: string;
    message: string;
    requestId: string;
  };
}
