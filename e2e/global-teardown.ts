/** Stops the isolated test stack started in global-setup.ts. */
import { readFileSync, rmSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const PIDS_FILE = path.join(ROOT, '.tmp', 'e2e-pids.json');

function stop(pid?: number): void {
  if (!pid) return;
  try {
    // Negative pid → kill the detached process group.
    process.kill(-pid, 'SIGTERM');
  } catch {
    try {
      process.kill(pid, 'SIGTERM');
    } catch {
      /* already gone */
    }
  }
}

export default function globalTeardown(): void {
  try {
    const pids = JSON.parse(readFileSync(PIDS_FILE, 'utf-8')) as {
      synodl?: number;
      synomock?: number;
    };
    stop(pids.synodl);
    stop(pids.synomock);
    rmSync(PIDS_FILE, { force: true });
  } catch {
    /* nothing to tear down */
  }
}
