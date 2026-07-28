import { describe, expect, it } from 'vitest';
import { whatsNew } from './release-notes';

describe('whatsNew', () => {
  it('shows only incoming changes the running build does not already have', () => {
    const running = [{ sha: 'aaa', subject: 'feat(tasks): old thing' }];
    const incoming = [
      { sha: 'ccc', subject: 'fix(browser): new fix' },
      { sha: 'bbb', subject: 'feat(app): new feature' },
      { sha: 'aaa', subject: 'feat(tasks): old thing' }, // already running → excluded
    ];
    expect(whatsNew(incoming, running)).toEqual(['new fix', 'new feature']);
  });

  it('drops non-user-facing types and strips the type(scope) prefix', () => {
    const incoming = [
      { sha: '1', subject: 'feat(tasks): pause with a swipe' },
      { sha: '2', subject: 'chore(deps): bump vite' }, // internal → dropped
      { sha: '3', subject: 'ci: tweak workflow' }, // internal → dropped
      { sha: '4', subject: 'fix: a plain fix' },
      { sha: '5', subject: 'perf(app)!: faster boot' },
      { sha: '6', subject: 'security(server): patch' },
    ];
    expect(whatsNew(incoming, [])).toEqual([
      'pause with a swipe',
      'a plain fix',
      'faster boot',
      'patch',
    ]);
  });

  it('is empty when every incoming change is already running', () => {
    const notes = [{ sha: 'x', subject: 'feat: shipped' }];
    expect(whatsNew(notes, notes)).toEqual([]);
  });
});
