export interface CaptureRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

interface SourceRect {
  sx: number;
  sy: number;
  sw: number;
  sh: number;
}

export interface MultiCropResult {
  tightCrop: string;
  contextCrop: string;
}

export function calculateObjectCoverSourceRect(
  videoSize: { width: number; height: number },
  renderedRect: DOMRectReadOnly,
  captureRect: CaptureRect
): SourceRect {
  const scale = Math.max(renderedRect.width / videoSize.width, renderedRect.height / videoSize.height);
  const displayedWidth = videoSize.width * scale;
  const displayedHeight = videoSize.height * scale;
  const offsetX = renderedRect.left + (renderedRect.width - displayedWidth) / 2;
  const offsetY = renderedRect.top + (renderedRect.height - displayedHeight) / 2;

  const sx = clamp((captureRect.x - offsetX) / scale, 0, videoSize.width);
  const sy = clamp((captureRect.y - offsetY) / scale, 0, videoSize.height);
  const right = clamp((captureRect.x + captureRect.width - offsetX) / scale, 0, videoSize.width);
  const bottom = clamp((captureRect.y + captureRect.height - offsetY) / scale, 0, videoSize.height);

  return {
    sx,
    sy,
    sw: Math.max(1, right - sx),
    sh: Math.max(1, bottom - sy),
  };
}

export function cropImage(video: HTMLVideoElement, frameRect: CaptureRect, videoRect: DOMRectReadOnly): string {
  if (video.videoWidth === 0 || video.videoHeight === 0) {
    throw new Error('Camera video is not ready yet');
  }

  const source = calculateObjectCoverSourceRect(
    { width: video.videoWidth, height: video.videoHeight },
    videoRect,
    frameRect
  );
  const canvas = document.createElement('canvas');
  const targetWidth = Math.min(source.sw, 1024);
  const targetHeight = Math.max(1, Math.round(targetWidth * (source.sh / source.sw)));

  canvas.width = Math.round(targetWidth);
  canvas.height = targetHeight;

  const ctx = canvas.getContext('2d');
  if (!ctx) {
    throw new Error('Canvas 2D context not available');
  }

  ctx.drawImage(video, source.sx, source.sy, source.sw, source.sh, 0, 0, canvas.width, canvas.height);

  return canvas.toDataURL('image/jpeg', 0.82);
}

export function cropImageMultiVariant(
  video: HTMLVideoElement,
  tightRect: CaptureRect,
  videoRect: DOMRectReadOnly
): MultiCropResult {
  return {
    tightCrop: cropImage(video, tightRect, videoRect),
    contextCrop: cropImage(video, expandRect(tightRect, videoRect, 0.18), videoRect),
  };
}

function expandRect(rect: CaptureRect, bounds: DOMRectReadOnly, ratio: number): CaptureRect {
  const growX = rect.width * ratio;
  const growY = rect.height * ratio;
  const x = clamp(rect.x - growX, bounds.left, bounds.right);
  const y = clamp(rect.y - growY, bounds.top, bounds.bottom);
  const right = clamp(rect.x + rect.width + growX, bounds.left, bounds.right);
  const bottom = clamp(rect.y + rect.height + growY, bounds.top, bounds.bottom);

  return {
    x,
    y,
    width: Math.max(1, right - x),
    height: Math.max(1, bottom - y),
  };
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
