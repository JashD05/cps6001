/// <reference types="vite/client" />

// ---------------------------------------------------------------------------
// Vite Client Types – enables TypeScript to understand import.meta.env
// ---------------------------------------------------------------------------

interface ImportMetaEnv {
  /** Whether the app is running in development mode */
  readonly DEV: boolean;
  /** Whether the app is running in production mode */
  readonly PROD: boolean;
  /** The mode the app is running in (e.g. 'development', 'production', 'test') */
  readonly MODE: string;
  /** The base URL the app is served from */
  readonly BASE_URL: string;
  /** Whether the app is running in SSR mode */
  readonly SSR: boolean;
  /** API base URL (defaults to /api/v1) */
  readonly VITE_API_BASE_URL: string;
  /** Enable performance measurement in dev */
  readonly VITE_MEASURE_PERF: string;
  /** Application title */
  readonly VITE_APP_TITLE: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
