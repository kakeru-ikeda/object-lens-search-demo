import { useCallback, useRef, useState } from 'react';
import { recognizeAndSearch, recognizeAndSearchStream } from '../lib/apiClient';
import { ImageCrops, ImageInput, RecognizeSearchResponse, StreamProgressEvent } from '../types';

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


export function useRecognizeSearchStream() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [data, setData] = useState<RecognizeSearchResponse | null>(null);
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
    setError(null);
    setEvents([]);
  }, [abort]);

  return { loading, error, data, events, startStream, abort, clearData };
}
