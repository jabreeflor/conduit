/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_CONDUIT_API?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
