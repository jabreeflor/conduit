// connect.ts — TypeScript surface around connect.mjs so the extension build
// stays type-checked while the test suite (using node:test) can import the
// pure-ESM source directly without a TS loader.

// @ts-ignore — module path resolved at runtime; tests use the .mjs directly.
export { probe } from './connect.mjs';

export type ConnectionState =
  | { kind: 'disconnected' }
  | { kind: 'connecting'; endpoint: string }
  | { kind: 'connected'; endpoint: string; version: string }
  | { kind: 'error'; endpoint: string; message: string };

export interface HealthResponse {
  status: string;
  version: string;
}

export interface ProbeOptions {
  endpoint: string;
  timeoutMs: number;
  fetchImpl?: typeof fetch;
}
