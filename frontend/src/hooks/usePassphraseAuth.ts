import { useCallback, useEffect, useMemo, useState } from 'react';
import { passphraseAuthConfig } from '../config/auth';

type AuthStatus = 'checking' | 'authenticated' | 'unauthenticated' | 'unconfigured';

interface PassphraseAuthState {
  status: AuthStatus;
  verifying: boolean;
  error: string | null;
  verify: (passphrase: string) => Promise<boolean>;
  logout: () => void;
}

const STORAGE_KEY = 'object_lens_passphrase_auth';

export function usePassphraseAuth(): PassphraseAuthState {
  const configFingerprint = useMemo(() => {
    if (!passphraseAuthConfig) return null;
    return `${passphraseAuthConfig.saltHex}:${passphraseAuthConfig.hashHex}:${passphraseAuthConfig.iterations}`;
  }, []);

  const [status, setStatus] = useState<AuthStatus>('checking');
  const [verifying, setVerifying] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!passphraseAuthConfig || !configFingerprint) {
      setStatus('unconfigured');
      return;
    }

    setStatus(sessionStorage.getItem(STORAGE_KEY) === configFingerprint ? 'authenticated' : 'unauthenticated');
  }, [configFingerprint]);

  const verify = useCallback(async (passphrase: string) => {
    if (!passphraseAuthConfig || !configFingerprint) {
      setStatus('unconfigured');
      setError('パスフレーズ認証が未設定です。');
      return false;
    }

    const candidate = passphrase.trim();
    if (!candidate) {
      setError('パスフレーズを入力してください。');
      return false;
    }

    setVerifying(true);
    setError(null);

    try {
      const verified = await verifyPassphrase(candidate, passphraseAuthConfig);
      if (!verified) {
        setStatus('unauthenticated');
        setError('パスフレーズが一致しません。');
        return false;
      }

      sessionStorage.setItem(STORAGE_KEY, configFingerprint);
      setStatus('authenticated');
      return true;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'パスフレーズの検証に失敗しました。';
      setError(message);
      setStatus('unauthenticated');
      return false;
    } finally {
      setVerifying(false);
    }
  }, [configFingerprint]);

  const logout = useCallback(() => {
    sessionStorage.removeItem(STORAGE_KEY);
    setStatus(passphraseAuthConfig ? 'unauthenticated' : 'unconfigured');
    setError(null);
  }, []);

  return { status, verifying, error, verify, logout };
}

async function verifyPassphrase(passphrase: string, config: NonNullable<typeof passphraseAuthConfig>): Promise<boolean> {
  const derivedHash = await derivePassphraseHash(passphrase, config.saltHex, config.iterations);
  return timingSafeEqualHex(derivedHash, config.hashHex);
}

async function derivePassphraseHash(passphrase: string, saltHex: string, iterations: number): Promise<string> {
  const keyMaterial = await crypto.subtle.importKey(
    'raw',
    new TextEncoder().encode(passphrase),
    'PBKDF2',
    false,
    ['deriveBits'],
  );

  const bits = await crypto.subtle.deriveBits(
    {
      name: 'PBKDF2',
      hash: 'SHA-256',
      salt: hexToArrayBuffer(saltHex),
      iterations,
    },
    keyMaterial,
    256,
  );

  return bytesToHex(new Uint8Array(bits));
}

function hexToArrayBuffer(hex: string): ArrayBuffer {
  if (!/^[0-9a-f]+$/i.test(hex) || hex.length % 2 !== 0) {
    throw new Error('認証設定の salt が不正です。');
  }

  const bytes = new Uint8Array(hex.length / 2);
  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = Number.parseInt(hex.slice(index * 2, index * 2 + 2), 16);
  }
  return bytes.buffer;
}

function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

function timingSafeEqualHex(left: string, right: string): boolean {
  const normalizedLeft = left.toLowerCase();
  const normalizedRight = right.toLowerCase();
  if (normalizedLeft.length !== normalizedRight.length) return false;

  let diff = 0;
  for (let index = 0; index < normalizedLeft.length; index += 1) {
    diff |= normalizedLeft.charCodeAt(index) ^ normalizedRight.charCodeAt(index);
  }
  return diff === 0;
}
