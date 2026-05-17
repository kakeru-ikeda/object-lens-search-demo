import { useCallback, useRef, useState } from 'react';
import { recognizeAndSearch, recognizeAndSearchStream } from '../lib/apiClient';
import { ImageCrops, ImageInput, RecognizeSearchResponse, StreamProgressEvent, NormalizedSearchResult } from '../types';

export function useRecognizeSearch() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [data, setData] = useState<RecognizeSearchResponse | null>(null);

  const fetchSearch = useCallback(async (imageBase64: string | ImageCrops) => {
    setLoading(true);
    setError(null);
    try {
      const imagePayload = typeof imageBase64 === 'string' ? { imageBase64 } : { crops: imageBase64 };
      const result = await recognizeAndSearch({
        ...imagePayload,
        language: 'ja',
        options: { maxSearchResults: 5, enableMultiCrop: typeof imageBase64 !== 'string' },
      });
      setData(result);
    } catch (err) {
      if (err instanceof Error) {
        setError(err);
      } else {
        setError(new Error('Unknown error'));
      }
    } finally {
      setLoading(false);
    }
  }, []);

  const clearData = useCallback(() => {
    setData(null);
    setError(null);
  }, []);

  return { loading, error, data, fetchSearch, clearData };
}


export interface PartialData {
  hypothesis?: string;
  query?: string;
  searchResults?: NormalizedSearchResult[];
}

export function useRecognizeSearchStream() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [data, setData] = useState<RecognizeSearchResponse | null>(null);
  const [partialData, setPartialData] = useState<PartialData | null>(null);
  const [events, setEvents] = useState<StreamProgressEvent[]>([]);
  const abortRef = useRef<AbortController | null>(null);

  const startStream = useCallback(async (images: ImageInput[]) => {
    if (images.length === 0) return;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    setError(null);
    setData(null);
    setPartialData({});
    setEvents([]);
    try {
      const result = await recognizeAndSearchStream(
        {
          images,
          language: 'ja',
          options: { maxSearchResults: 5, enableMultiCrop: true, maxImages: 5, stream: true },
        },
        {
          signal: controller.signal,
          onEvent: (event) => {
            setEvents((current) => {
              if (current.some((item) => item.requestId === event.requestId && item.sequence >= event.sequence)) {
                return current;
              }
              return [...current, event];
            });
            
            if (event.payload) {
              setPartialData((current) => ({
                ...current,
                ...(event.payload?.hypothesis ? { hypothesis: event.payload.hypothesis } : {}),
                ...(event.payload?.query ? { query: event.payload.query } : {}),
                ...(event.payload?.searchResults ? { searchResults: mergeSearchResults(current?.searchResults ?? [], event.payload.searchResults) } : {}),
              }));
            }

            if (event.stage === 'final' && event.payload?.response) {
              setData(event.payload.response);
            }
          },
        },
      );
      setData(result);
    } catch (err) {
      if (controller.signal.aborted) return;
      if (err instanceof Error) {
        setError(err);
      } else {
        setError(new Error('Unknown streaming error'));
      }
    } finally {
      if (!controller.signal.aborted) {
        setLoading(false);
      }
    }
  }, []);

  const abort = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setLoading(false);
  }, []);

  const clearData = useCallback(() => {
    abort();
    setData(null);
    setPartialData(null);
    setError(null);
    setEvents([]);
  }, [abort]);

  return { loading, error, data, partialData, events, startStream, abort, clearData };
}

function mergeSearchResults(current: NormalizedSearchResult[], incoming: NormalizedSearchResult[]) {
  const merged: NormalizedSearchResult[] = [];
  const seen = new Set<string>();
  for (const result of [...current, ...incoming]) {
    const key = searchResultIdentity(result);
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    merged.push(result);
  }
  return merged;
}

function searchResultIdentity(result: NormalizedSearchResult) {
  const url = result.url.trim().toLowerCase();
  if (url) {
    return url;
  }
  return `${result.id}:${result.title}:${result.snippet}`.trim().toLowerCase();
}
