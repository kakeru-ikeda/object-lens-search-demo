import { useState, useEffect, useRef } from 'react';

function createCameraError(error: unknown): Error {
  if (error instanceof Error) {
    if (error.name === 'NotAllowedError') {
      return new Error('カメラ権限が拒否されました。ブラウザ設定でカメラ権限を許可してください。');
    }
    if (error.name === 'NotFoundError') {
      return new Error('この端末で利用可能なカメラが見つかりませんでした。カメラ付き端末で開いてください。');
    }
    if (error.name === 'NotReadableError') {
      return new Error('カメラを開始できませんでした。他のアプリがカメラを使用していないか確認してください。');
    }
    return new Error(`${error.name}: ${error.message}`);
  }

  return new Error('カメラの起動に失敗しました。');
}

function getUnavailableCameraReason(): Error | null {
  if (!window.isSecureContext) {
    return new Error('カメラはHTTPSまたはlocalhostでのみ利用できます。スマホで確認する場合はHTTPS URLで開いてください。');
  }
  if (!navigator.mediaDevices?.getUserMedia) {
    return new Error('このブラウザはカメラAPIに対応していないか、現在の接続ではカメラAPIを利用できません。');
  }
  return null;
}

export function useCamera() {
  const [stream, setStream] = useState<MediaStream | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    const unavailableReason = getUnavailableCameraReason();
    if (unavailableReason) {
      setError(unavailableReason);
      return undefined;
    }

    let activeStream: MediaStream | null = null;
    let cancelled = false;

    navigator.mediaDevices
      .getUserMedia({
        video: { facingMode: { ideal: 'environment' } },
        audio: false,
      })
      .then((nextStream) => {
        if (cancelled) {
          nextStream.getTracks().forEach((track) => track.stop());
          return;
        }

        activeStream = nextStream;
        setStream(nextStream);
        if (videoRef.current) {
          videoRef.current.srcObject = nextStream;
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(createCameraError(err));
        }
      });

    return () => {
      cancelled = true;
      if (videoRef.current) {
        videoRef.current.srcObject = null;
      }
      if (activeStream) {
        activeStream.getTracks().forEach((track) => track.stop());
      }
    };
  }, []);

  return { stream, error, videoRef };
}
