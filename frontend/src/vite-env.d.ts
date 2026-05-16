/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string;
  readonly VITE_AUTH_PASSPHRASE_HASH?: string;
  readonly VITE_AUTH_PASSPHRASE_SALT?: string;
  readonly VITE_AUTH_PASSPHRASE_ITERATIONS?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
