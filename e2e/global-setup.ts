/**
 * e2e global setup: brings up an ISOLATED test stack so the suite is hermetic
 * and repeatable — no real NAS, no Docker, no shared state with `make start`.
 *
 *   1. build cmd/synomock and cmd/synodl into .tmp/
 *   2. start the mock DSM on :8292 (fixed accounts + seedable fixtures)
 *   3. start synodl on :8281 with SYNO_URL pointed at the mock
 *
 * Plus a SECOND, stateful stack for the features that only exist in stateful
 * mode — accounts, and the download sources (spec 0007). It is deliberately
 * additive: the stateless stack above is untouched, so the specs that exercise
 * stateless behaviour (the NAS login states, the "hidden in stateless mode"
 * assertions) keep testing exactly what they always did.
 *
 *   4. start a TLS mock DSM on :8294 (stateful synodl always dials https://)
 *   5. start a stateful synodl on :8283, built with the `sourcemock` tag so a
 *      download source can point at the mock's fake sites — no credentials
 *   6. run its first-run setup once, so specs start from a signed-in-able state
 *
 * Playwright's `webServer` serves a test vite on :5274 proxied at the stateless
 * backend, and another on :5275 proxied at the stateful one. Torn down in
 * global-teardown.ts. Requires Go on PATH.
 */
import { spawn, execSync } from 'node:child_process';
import { mkdirSync, openSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SERVER = path.join(ROOT, 'server');
const TMP = path.join(ROOT, '.tmp');
const PIDS_FILE = path.join(TMP, 'e2e-pids.json');

// Defaults match CI; override with SYNODL_E2E_PORT / SYNODL_E2E_MOCK_PORT when
// taken locally. Keep in sync with playwright.config.ts (SYNODL_PROXY_TARGET)
// and e2e/helpers.ts (MOCK), which read the same env.
const SYNODL_PORT = Number(process.env.SYNODL_E2E_PORT) || 8281;
const MOCK_PORT = Number(process.env.SYNODL_E2E_MOCK_PORT) || 8292;
// The stateful pair. Separate ports so both stacks run side by side and neither
// spec set can disturb the other's fixtures.
const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const SF_MOCK_PORT = Number(process.env.SYNODL_E2E_SF_MOCK_PORT) || 8294;
export const STATEFUL_ADMIN = { username: 'e2eadmin', password: 'e2e-admin-password' };

async function waitFor(url: string, timeoutMs = 30_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      /* not up yet */
    }
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${url}`);
    await new Promise((r) => setTimeout(r, 300));
  }
}

function freePort(port: number): void {
  try {
    const pids = execSync(`lsof -nP -iTCP:${port} -sTCP:LISTEN -t || true`).toString().trim();
    for (const pid of pids.split('\n').filter(Boolean)) {
      try {
        process.kill(Number(pid), 'SIGKILL');
      } catch {
        /* gone */
      }
    }
  } catch {
    /* lsof missing - ignore */
  }
}

function start(bin: string, logName: string, env: Record<string, string>): number {
  const log = openSync(path.join(TMP, logName), 'w');
  const child = spawn(bin, [], {
    cwd: SERVER,
    detached: true,
    stdio: ['ignore', log, log],
    env: { ...process.env, ...env },
  });
  child.unref();
  return child.pid!;
}

export default async function globalSetup(): Promise<void> {
  mkdirSync(TMP, { recursive: true });

  // Clean any stale test stack from a previous aborted run.
  freePort(SYNODL_PORT);
  freePort(MOCK_PORT);
  freePort(SF_PORT);
  freePort(SF_MOCK_PORT);

  // 1. Build both test binaries.
  execSync(`go build -o "${TMP}/synomock-e2e" ./cmd/synomock`, { cwd: SERVER, stdio: 'inherit' });
  execSync(`go build -o "${TMP}/synodl-e2e" ./cmd/synodl`, { cwd: SERVER, stdio: 'inherit' });
  // The stateful binary carries the `sourcemock` tag, which is the ONLY way a
  // download source can be pointed at a fake site. A release build never passes
  // it, so this capability exists in tests and nowhere else.
  execSync(`go build -tags sourcemock -o "${TMP}/synodl-e2e-sf" ./cmd/synodl`, {
    cwd: SERVER,
    stdio: 'inherit',
  });

  // 2. Mock DSM first (synodl discovers its API table lazily, but a healthy
  //    mock from the start keeps the first login snappy and deterministic).
  const mockPid = start(`${TMP}/synomock-e2e`, 'synomock-e2e.log', {
    MOCK_PORT: String(MOCK_PORT),
  });
  await waitFor(`http://localhost:${MOCK_PORT}/webapi/query.cgi`);

  // 3. The proxy under test.
  const synodlPid = start(`${TMP}/synodl-e2e`, 'synodl-e2e.log', {
    ENV: 'dev',
    PORT: String(SYNODL_PORT),
    SYNO_URL: `http://localhost:${MOCK_PORT}`,
    ALLOWED_ORIGINS: `http://localhost:5274,http://localhost:${SYNODL_PORT}`,
    // e2e drives repeated logins across specs; don't trip the brute-force guard.
    LOGIN_PER_MINUTE: '1000',
  });
  await waitFor(`http://localhost:${SYNODL_PORT}/healthz`);

  // 4. The stateful stack's mock, over TLS — stateful synodl always dials
  //    https://, the same as it would a self-signed NAS.
  const sfMockPid = start(`${TMP}/synomock-e2e`, 'synomock-e2e-sf.log', {
    MOCK_PORT: String(SF_MOCK_PORT),
    MOCK_TLS: '1',
  });
  await waitForTLS(`https://localhost:${SF_MOCK_PORT}/webapi/query.cgi`);

  // 5. The stateful proxy under test. A fresh DATA_DIR per run keeps it
  //    hermetic: no state survives from a previous run.
  const sfData = path.join(TMP, 'e2e-sf-data');
  rmSync(sfData, { recursive: true, force: true });
  mkdirSync(sfData, { recursive: true });
  const sfPid = start(`${TMP}/synodl-e2e-sf`, 'synodl-e2e-sf.log', {
    ENV: 'dev',
    PORT: String(SF_PORT),
    SECRETS_KEY: 'e2e-only-secrets-key',
    DATA_DIR: sfData,
    SYNO_TLS_INSECURE: 'true',
    ALLOWED_ORIGINS: `http://localhost:5275,http://localhost:${SF_PORT}`,
    LOGIN_PER_MINUTE: '1000',
    // Point both drivers at the mock's fake sites so a source can be configured
    // with no real credentials.
    SOURCE_MOCK_ZARFILM: `https://localhost:${SF_MOCK_PORT}/mocksrc/zar`,
    SOURCE_MOCK_THIRTYNAMA: `https://localhost:${SF_MOCK_PORT}/mocksrc/tn`,
  });
  await waitFor(`http://localhost:${SF_PORT}/healthz`);

  // 6. First-run setup, so specs can sign in instead of each repeating a wizard.
  const setup = await fetch(`http://localhost:${SF_PORT}/v1/setup`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      nasAddress: 'localhost',
      nasPort: SF_MOCK_PORT,
      nasTlsVerify: false,
      nasAccount: 'admin',
      nasPassword: 'secret',
      otp: '',
      adminUsername: STATEFUL_ADMIN.username,
      adminPassword: STATEFUL_ADMIN.password,
    }),
  });
  if (!setup.ok) {
    throw new Error(`stateful setup failed: ${setup.status} ${await setup.text()}`);
  }

  writeFileSync(
    PIDS_FILE,
    JSON.stringify({ synodl: synodlPid, synomock: mockPid, synodlSf: sfPid, synomockSf: sfMockPid }),
  );
}

/**
 * waitForTLS is waitFor for the self-signed mock. Node's fetch has no
 * per-request "skip verification", so this drops verification for the duration
 * of the poll and restores it — the whole point of the mock being self-signed is
 * that it mirrors a self-signed NAS, so refusing it here would be testing the
 * wrong thing.
 */
async function waitForTLS(url: string, timeoutMs = 30_000): Promise<void> {
  const prev = process.env.NODE_TLS_REJECT_UNAUTHORIZED;
  process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';
  try {
    await waitFor(url, timeoutMs);
  } finally {
    if (prev === undefined) delete process.env.NODE_TLS_REJECT_UNAUTHORIZED;
    else process.env.NODE_TLS_REJECT_UNAUTHORIZED = prev;
  }
}
