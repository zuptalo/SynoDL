import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from './api';

// Regression coverage for the HTTP wrapper's response handling. The bug (fix/2001):
// POST /v1/tasks returns 201 with an EMPTY body, but request() only special-cased
// 204, so it called resp.json() on the empty body → SyntaxError → surfaced as the
// false "Could not reach the server." while the task was actually created.

afterEach(() => {
  vi.restoreAllMocks();
});

function mockFetch(resp: Response): void {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(resp));
}

describe('api request() response handling', () => {
  it('resolves (does not throw) on a 201 Created with an empty body', async () => {
    mockFetch(new Response('', { status: 201 }));
    // Before the fix this rejects with a JSON parse error.
    await expect(api.createTaskURIs(['http://mirror.example/one.iso'])).resolves.toBeUndefined();
  });

  it('resolves on a 204 No Content (pause/resume/delete path)', async () => {
    mockFetch(new Response(null, { status: 204 }));
    await expect(api.pauseTasks(['dbid_1'])).resolves.toBeUndefined();
  });

  it('still parses a non-empty JSON success body', async () => {
    mockFetch(
      new Response(JSON.stringify({ tasks: [], stats: { downloadSpeed: 1, uploadSpeed: 2 } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    await expect(api.tasks()).resolves.toEqual({ tasks: [], stats: { downloadSpeed: 1, uploadSpeed: 2 } });
  });

  it('maps a genuine HTTP error body to a typed ApiError', async () => {
    mockFetch(
      new Response(JSON.stringify({ error: 'nas_unreachable' }), {
        status: 502,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    await expect(api.createTaskURIs(['http://x/y.iso'])).rejects.toMatchObject({
      code: 'nas_unreachable',
      status: 502,
    });
    await expect(api.createTaskURIs(['http://x/y.iso'])).rejects.toBeInstanceOf(ApiError);
  });

  it('propagates a true network failure (fetch rejects) as a non-ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));
    await expect(api.createTaskURIs(['http://x/y.iso'])).rejects.not.toBeInstanceOf(ApiError);
  });
});
