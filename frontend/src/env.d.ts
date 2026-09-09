/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_LINE_LIFF_ID?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
