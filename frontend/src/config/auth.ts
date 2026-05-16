export interface PassphraseAuthConfig {
  hashHex: string;
  saltHex: string;
  iterations: number;
}

const DEFAULT_ITERATIONS = 600_000;

const hashHex = import.meta.env.VITE_AUTH_PASSPHRASE_HASH?.trim() ?? '';
const saltHex = import.meta.env.VITE_AUTH_PASSPHRASE_SALT?.trim() ?? '';
const iterationValue = import.meta.env.VITE_AUTH_PASSPHRASE_ITERATIONS?.trim();
const iterations = iterationValue ? Number(iterationValue) : DEFAULT_ITERATIONS;

export const passphraseAuthConfig: PassphraseAuthConfig | null = hashHex && saltHex
  ? {
      hashHex,
      saltHex,
      iterations: Number.isFinite(iterations) && iterations > 0 ? iterations : DEFAULT_ITERATIONS,
    }
  : null;
