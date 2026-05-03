// connect.mjs — minimal connection probe to a running Conduit instance.
// Pure ESM so the Node test runner can import it without a TypeScript
// loader. The TypeScript shim in connect.ts re-exports from here so the
// extension build keeps full type checking.

/**
 * @typedef {{ kind: 'disconnected' }
 *   | { kind: 'connecting', endpoint: string }
 *   | { kind: 'connected', endpoint: string, version: string }
 *   | { kind: 'error', endpoint: string, message: string }} ConnectionState
 */

/**
 * @param {{ endpoint: string, timeoutMs: number, fetchImpl?: typeof fetch }} opts
 * @returns {Promise<ConnectionState>}
 */
export async function probe(opts) {
  const { endpoint, timeoutMs } = opts;
  const fetchImpl = opts.fetchImpl ?? fetch;

  const url = `${endpoint.replace(/\/$/, '')}/v1/healthz`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const res = await fetchImpl(url, { signal: controller.signal });
    if (!res.ok) {
      return { kind: 'error', endpoint, message: `HTTP ${res.status}` };
    }
    const body = await res.json();
    if (!body || typeof body.status !== 'string') {
      return { kind: 'error', endpoint, message: 'malformed /v1/healthz body' };
    }
    return {
      kind: 'connected',
      endpoint,
      version: body.version ?? 'unknown',
    };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    if (message.includes('aborted')) {
      return { kind: 'error', endpoint, message: `timed out after ${timeoutMs}ms` };
    }
    return { kind: 'error', endpoint, message };
  } finally {
    clearTimeout(timer);
  }
}
