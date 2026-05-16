import { useState, useCallback } from 'react';
import { useCamera } from '../hooks/useCamera';
import { useRecognizeSearchStream } from '../hooks/useRecognizeSearch';
import { cropImageMultiVariant } from '../lib/cropImage';
import { CaptureOverlay } from './CaptureOverlay';
import { ResultPanel } from './ResultPanel';
import { Camera, Loader2, Search, Trash2, X } from 'lucide-react';
import { ImageInput, StreamProgressEvent } from '../types';

interface CapturedImage extends ImageInput {
  id: string;
  preview: string;
}

export function CameraView() {
  const { stream, error: cameraError, videoRef } = useCamera();
  const { loading, error: searchError, data, events, startStream, clearData } = useRecognizeSearchStream();
  const [captureRect, setCaptureRect] = useState<{ x: number; y: number; width: number; height: number } | null>(null);
  const [images, setImages] = useState<CapturedImage[]>([]);

  const handleCapture = useCallback(() => {
    if (!videoRef.current || !captureRect || images.length >= 5) return;

    const video = videoRef.current;
    const videoRect = video.getBoundingClientRect();

    try {
      const crops = cropImageMultiVariant(video, captureRect, videoRect);
      const id = `image_${Date.now()}_${images.length + 1}`;
      setImages((current) => [
        ...current,
        {
          id,
          role: current.length === 0 ? 'primary' : 'supporting',
          crops,
          preview: crops.tightCrop,
        },
      ]);
    } catch (err) {
      console.error('Failed to crop image', err);
    }
  }, [captureRect, images.length, videoRef]);

  const handleSearch = useCallback(() => {
    startStream(images.map(({ preview: _preview, ...image }) => image));
  }, [images, startStream]);

  const removeImage = useCallback((id: string) => {
    setImages((current) => current.filter((image) => image.id !== id).map((image, index) => ({ ...image, role: index === 0 ? 'primary' : 'supporting' })));
  }, []);

  const reset = useCallback(() => {
    clearData();
    setImages([]);
  }, [clearData]);

  if (cameraError) {
    return (
      <div className="flex items-center justify-center app-viewport bg-black text-white p-6">
        <div className="max-w-sm space-y-3 text-center">
          <p className="text-lg font-semibold">カメラを起動できません</p>
          <p className="text-sm text-white/80">{cameraError.message}</p>
          <p className="text-xs text-white/60">
            スマホでPCのローカル開発サーバーを見る場合は、HTTPSのトンネルURL（ngrok / cloudflared など）を使ってください。
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="relative flex flex-col app-viewport bg-black overflow-hidden">
      <div className="relative flex-1">
        <video ref={videoRef} autoPlay playsInline muted className="absolute inset-0 w-full h-full object-cover" />
        {stream && <CaptureOverlay onCaptureRectChange={setCaptureRect} />}
      </div>

      <div className="absolute top-4 inset-x-4 z-10 space-y-3">
        {images.length > 0 && <ImageTray images={images} onRemove={removeImage} />}
        {events.length > 0 && <StreamTimeline events={events} />}
      </div>

      <div className="absolute bottom-0 inset-x-0 flex flex-col items-center gap-2 pb-[max(1.5rem,env(safe-area-inset-bottom))] pt-3 bg-gradient-to-t from-black/85 via-black/55 to-transparent">
        {searchError && <div className="px-4 py-2 bg-red-500 text-white rounded-lg text-sm max-w-xs text-center">{searchError.message}</div>}

        <div className="text-xs text-white/80 bg-black/40 rounded-full px-3 py-1">
          {images.length}/5 images captured {images.length > 1 ? '· coverage increased' : ''}
        </div>

        <div className="flex items-center gap-4">
          <button
            onClick={handleCapture}
            disabled={!stream || loading || images.length >= 5}
            className="relative flex items-center justify-center w-16 h-16 sm:w-20 sm:h-20 bg-white rounded-full disabled:opacity-50 hover:bg-neutral-100 transition-colors shadow-lg shadow-black/30"
            aria-label="画像を追加"
          >
            <Camera className="w-7 h-7 sm:w-8 sm:h-8 text-neutral-900" />
          </button>

          <button
            onClick={handleSearch}
            disabled={images.length === 0 || loading}
            className="flex items-center gap-2 px-4 py-2.5 sm:px-5 sm:py-3 bg-blue-500 text-white rounded-full disabled:opacity-50 hover:bg-blue-600 transition-colors shadow-lg shadow-black/20"
            aria-label="複数画像で検索する"
          >
            {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : <Search className="w-5 h-5" />}
            Search with {images.length}
          </button>
        </div>
      </div>

      {data && (
        <div className="absolute inset-0 z-50 flex items-end bg-black/60 backdrop-blur-sm transition-opacity" role="dialog" aria-modal="true" aria-label="検索結果">
          <button
            type="button"
            className="absolute [top:max(1rem,env(safe-area-inset-top))] [right:max(1rem,env(safe-area-inset-right))] z-[60] p-2 text-white bg-black/50 rounded-full shadow-lg backdrop-blur transition-colors hover:bg-black/70"
            onClick={reset}
            aria-label="検索結果を閉じる"
          >
            <X className="w-5 h-5" />
          </button>
          <ResultPanel data={data} onClose={reset} />
        </div>
      )}
    </div>
  );
}

function ImageTray({ images, onRemove }: { images: CapturedImage[]; onRemove: (id: string) => void }) {
  return (
    <div className="flex gap-2 overflow-x-auto rounded-2xl bg-black/45 p-2 backdrop-blur">
      {images.map((image, index) => (
        <div key={image.id} className="relative shrink-0">
          <img src={image.preview} alt={`captured ${index + 1}`} className="h-16 w-16 rounded-xl object-cover border border-white/30" />
          <span className="absolute left-1 top-1 rounded-full bg-black/70 px-1.5 py-0.5 text-[10px] text-white">{index === 0 ? 'primary' : index + 1}</span>
          <button onClick={() => onRemove(image.id)} className="absolute -right-1 -top-1 rounded-full bg-red-500 p-1 text-white" aria-label="画像を削除">
            <Trash2 className="h-3 w-3" />
          </button>
        </div>
      ))}
    </div>
  );
}

function StreamTimeline({ events }: { events: StreamProgressEvent[] }) {
  return (
    <div className="rounded-2xl bg-black/55 p-3 text-white backdrop-blur">
      <p className="mb-2 text-xs font-semibold">Evidence-backed progress</p>
      <div className="space-y-1.5">
        {events.slice(-5).map((event) => (
          <div key={`${event.requestId}-${event.sequence}`} className="flex items-center justify-between gap-3 text-xs">
            <span className="truncate">{stageLabel(event.stage)} · {event.message}</span>
            <span className="shrink-0 text-white/60">{Math.round(event.elapsedMs / 100) / 10}s</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function stageLabel(stage: string) {
  const labels: Record<string, string> = {
    request_received: 'request',
    vision_started: 'vision',
    vision_completed: 'vision',
    recognition_started: 'recognition',
    recognition_completed: 'recognition',
    search_started: 'search',
    search_completed: 'search',
    summary_started: 'summary',
    summary_completed: 'summary',
    final: 'final',
  };
  return labels[stage] ?? stage;
}
