export interface RecognizedObject {
  objectName: string;
  description: string;
  searchQuery: string;
  confidence: 'low' | 'medium' | 'high';
  needsMoreContext: boolean;
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

export interface RecognizeSearchResponse {
  requestId: string;
  recognizedObject: RecognizedObject;
  search: {
    provider: string;
    query: string;
    results: NormalizedSearchResult[];
  };
  summary: SearchSummary;
  meta: {
    llmProvider: string;
    searchProvider: string;
    elapsedMs: number;
  };
}

export interface RecognizeSearchRequest {
  imageBase64: string;
  language?: string;
  options?: {
    maxSearchResults?: number;
  };
}

export interface ErrorResponse {
  error: {
    code: string;
    message: string;
    requestId: string;
  };
}
