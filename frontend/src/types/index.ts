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
  responseVersion?: number;
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
  inputSummary?: InputSummary;
  imageAnalyses?: ImageAnalysis[];
  evidenceFusion?: EvidenceFusion;
}

export interface ImageInput {
  id?: string;
  role?: 'primary' | 'supporting' | string;
  imageBase64?: string;
  crops?: ImageCrops;
}

export interface InputSummary {
  imageCount: number;
  primaryImageId: string;
  imageIds: string[];
  roles?: string[];
  mode: string;
}

export interface ImageAnalysis {
  imageId: string;
  role?: string;
  evidenceTypes?: string[];
  status: string;
  evidence?: VisualEvidence;
}

export interface EvidenceFusion {
  coverage: string;
  agreement: string;
  signals?: string[];
  primaryImageId: string;
}

export type StreamStage =
  | 'request_received'
  | 'vision_started'
  | 'vision_completed'
  | 'recognition_started'
  | 'recognition_completed'
  | 'search_started'
  | 'search_completed'
  | 'summary_started'
  | 'summary_completed'
  | 'final'
  | 'error'
  | string;

export interface StreamProgressEvent {
  requestId: string;
  sequence: number;
  stage: StreamStage;
  status: 'queued' | 'started' | 'completed' | 'warning' | 'error' | string;
  elapsedMs: number;
  imageId?: string;
  message: string;
  payload?: {
    response?: RecognizeSearchResponse;
    code?: string;
    [key: string]: unknown;
  };
}

export interface RecognizeSearchRequest {
  imageBase64?: string;
  crops?: ImageCrops;
  images?: ImageInput[];
  language?: string;
  options?: {
    maxSearchResults?: number;
    enableMultiCrop?: boolean;
    maxImages?: number;
    stream?: boolean;
  };
  inputSummary?: InputSummary;
  imageAnalyses?: ImageAnalysis[];
  evidenceFusion?: EvidenceFusion;
}

export interface ErrorResponse {
  error: {
    code: string;
    message: string;
    requestId: string;
  };
  inputSummary?: InputSummary;
  imageAnalyses?: ImageAnalysis[];
  evidenceFusion?: EvidenceFusion;
}
