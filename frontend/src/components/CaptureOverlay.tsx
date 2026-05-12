import { useCallback, useEffect, useRef } from 'react';
import { CaptureRect } from '../lib/cropImage';

interface CaptureOverlayProps {
  onCaptureRectChange: (rect: CaptureRect) => void;
}

export function CaptureOverlay({ onCaptureRectChange }: CaptureOverlayProps) {
  const overlayRef = useRef<HTMLDivElement>(null);

  const updateCaptureRect = useCallback(() => {
    if (!overlayRef.current) {
      return;
    }

    const rect = overlayRef.current.getBoundingClientRect();
    onCaptureRectChange({
      x: rect.left,
      y: rect.top,
      width: rect.width,
      height: rect.height,
    });
  }, [onCaptureRectChange]);

  useEffect(() => {
    updateCaptureRect();
    window.addEventListener('resize', updateCaptureRect);
    window.addEventListener('orientationchange', updateCaptureRect);

    return () => {
      window.removeEventListener('resize', updateCaptureRect);
      window.removeEventListener('orientationchange', updateCaptureRect);
    };
  }, [updateCaptureRect]);

  return (
    <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
      <div className="absolute inset-0 bg-black/40" />
      <div
        ref={overlayRef}
        className="relative w-64 h-64 border-2 border-white rounded-xl shadow-[0_0_0_9999px_rgba(0,0,0,0.4)]"
      >
        <div className="absolute inset-0 border border-white/30 rounded-xl" />
      </div>
    </div>
  );
}
