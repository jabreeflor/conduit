// Tests for the connection probe. Pure Node test runner — no VS Code
// runtime needed, so this works in plain CI.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { probe } from '../src/connect.mjs';

const fakeFetchOK = async (_url, _opts) => ({
  ok: true,
  status: 200,
  json: async () => ({ status: 'ok', version: '0.1.0' }),
});

const fakeFetch500 = async () => ({ ok: false, status: 500, json: async () => ({}) });

const fakeFetchMalformed = async () => ({
  ok: true,
  status: 200,
  json: async () => ({ no_status_here: true }),
});

const fakeFetchThrows = async () => {
  throw new Error('ECONNREFUSED');
};

test('probe returns connected on healthy /v1/healthz', async () => {
  const state = await probe({ endpoint: 'http://localhost:8923', timeoutMs: 100, fetchImpl: fakeFetchOK });
  assert.equal(state.kind, 'connected');
  assert.equal(state.version, '0.1.0');
});

test('probe returns error on non-2xx', async () => {
  const state = await probe({ endpoint: 'http://localhost:8923', timeoutMs: 100, fetchImpl: fakeFetch500 });
  assert.equal(state.kind, 'error');
  assert.match(state.message, /HTTP 500/);
});

test('probe returns error on malformed body', async () => {
  const state = await probe({ endpoint: 'http://localhost:8923', timeoutMs: 100, fetchImpl: fakeFetchMalformed });
  assert.equal(state.kind, 'error');
  assert.match(state.message, /malformed/);
});

test('probe returns error on connection refused', async () => {
  const state = await probe({ endpoint: 'http://localhost:8923', timeoutMs: 100, fetchImpl: fakeFetchThrows });
  assert.equal(state.kind, 'error');
  assert.match(state.message, /ECONNREFUSED/);
});
