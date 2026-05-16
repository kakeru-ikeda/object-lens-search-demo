import { ErrorResponse, RecognizeSearchRequest, RecognizeSearchResponse, StreamProgressEvent } from '../types';

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


interface RecognizeStreamOptions {
  signal?: AbortSignal;
  onEvent: (event: StreamProgressEvent) => void;
}

export async function recognizeAndSearchStream(req: RecognizeSearchRequest, options: RecognizeStreamOptions): Promise<RecognizeSearchResponse> {
  const baseUrl = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';
  const res = await fetch(`${baseUrl}/api/recognize-search-stream`, {
    method: 'POST',
    headers: {
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(req),
    signal: options.signal,
  });

  if (!res.ok) {
    const apiError = await readAPIError(res);
    throw new Error(apiError);
  }
  if (!res.body) {
    throw new Error('Streaming response body is unavailable');
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let finalResponse: RecognizeSearchResponse | null = null;

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const frames = buffer.split('\n\n');
    buffer = frames.pop() ?? '';
    for (const frame of frames) {
      const event = parseSSEFrame(frame);
      if (!event) continue;
      options.onEvent(event);
      if (event.stage === 'final' && event.payload?.response) {
        finalResponse = event.payload.response;
      }
      if (event.stage === 'error') {
        throw new Error(event.message || 'Streaming recognition failed');
      }
    }
  }

  if (buffer.trim()) {
    const event = parseSSEFrame(buffer);
    if (event) {
      options.onEvent(event);
      if (event.stage === 'final' && event.payload?.response) {
        finalResponse = event.payload.response;
      }
    }
  }

  if (!finalResponse) {
    throw new Error('Stream ended before final response');
  }
  return finalResponse;
}

function parseSSEFrame(frame: string): StreamProgressEvent | null {
  const dataLines = frame
    .split('\n')
    .map((line) => line.trimEnd())
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).trimStart());
  if (dataLines.length === 0) return null;
  return JSON.parse(dataLines.join('\n')) as StreamProgressEvent;
}
