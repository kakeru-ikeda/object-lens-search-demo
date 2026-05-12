import { useCallback, useState } from 'react';
import { recognizeAndSearch } from '../lib/apiClient';
import { RecognizeSearchResponse } from '../types';

export function useRecognizeSearch() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [data, setData] = useState<RecognizeSearchResponse | null>(null);

  const fetchSearch = useCallback(async (imageBase64: string) => {
    setLoading(true);
    setError(null);
    try {
      const result = await recognizeAndSearch({
        imageBase64,
        language: 'ja',
        options: { maxSearchResults: 5 },
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
