import { ErrorResponse, RecognizeSearchRequest, RecognizeSearchResponse } from '../types';

export async function recognizeAndSearch(req: RecognizeSearchRequest): Promise<RecognizeSearchResponse> {
  const baseUrl = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';
  const res = await fetch(`${baseUrl}/api/recognize-search`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(req),
  });

  if (!res.ok) {
    const apiError = await readAPIError(res);
    throw new Error(apiError);
  }

  return res.json();
}

async function readAPIError(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as ErrorResponse;
    if (body.error.message) {
      return body.error.requestId ? `${body.error.message} (requestId: ${body.error.requestId})` : body.error.message;
    }
  } catch {
    return `API error: ${res.status} ${res.statusText}`;
  }
  return `API error: ${res.status} ${res.statusText}`;
}
