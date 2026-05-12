import { useState, useCallback } from 'react';
import { useCamera } from '../hooks/useCamera';
import { useRecognizeSearch } from '../hooks/useRecognizeSearch';
import { cropImage } from '../lib/cropImage';
import { CaptureOverlay } from './CaptureOverlay';
import { ResultPanel } from './ResultPanel';
import { Camera, Loader2 } from 'lucide-react';

export function CameraView() {
  const { stream, error: cameraError, videoRef } = useCamera();
  const { loading, error: searchError, data, fetchSearch, clearData } = useRecognizeSearch();
  const [captureRect, setCaptureRect] = useState<{ x: number; y: number; width: number; height: number } | null>(null);

  const handleCapture = useCallback(() => {
    if (!videoRef.current || !captureRect) return;
    
    const video = videoRef.current;
    const videoRect = video.getBoundingClientRect();
    
    try {
      const imageBase64 = cropImage(video, captureRect, videoRect);
      fetchSearch(imageBase64);
    } catch (err) {
      console.error('Failed to crop image', err);
    }
  }, [captureRect, fetchSearch, videoRef]);

  if (cameraError) {
    return (
      <div className="flex items-center justify-center h-screen bg-black text-white p-6">
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
    <div className="relative flex flex-col h-screen bg-black overflow-hidden">
      <div className="relative flex-1">
        <video
          ref={videoRef}
          autoPlay
          playsInline
          muted
          className="absolute inset-0 w-full h-full object-cover"
        />
        {stream && <CaptureOverlay onCaptureRectChange={setCaptureRect} />}
      </div>

      <div className="absolute bottom-0 inset-x-0 flex flex-col items-center pb-8 pt-4 bg-gradient-to-t from-black/80 to-transparent">
        {searchError && (
          <div className="mb-4 px-4 py-2 bg-red-500 text-white rounded-lg text-sm max-w-xs text-center">
            {searchError.message}
          </div>
        )}
        
        <button
          onClick={handleCapture}
          disabled={!stream || loading}
          className="relative flex items-center justify-center w-20 h-20 bg-white rounded-full disabled:opacity-50 hover:bg-neutral-100 transition-colors"
          aria-label="検索する"
        >
          {loading ? (
            <Loader2 className="w-8 h-8 text-neutral-900 animate-spin" />
          ) : (
            <Camera className="w-8 h-8 text-neutral-900" />
          )}
        </button>
      </div>

      {data && (
        <div className="absolute inset-0 z-50 flex items-end bg-black/60 backdrop-blur-sm transition-opacity">
          <button 
            className="absolute top-4 right-4 p-2 text-white bg-black/40 rounded-full"
            onClick={clearData}
          >
            閉じる
          </button>
          <ResultPanel data={data} />
        </div>
      )}
    </div>
  );
}
