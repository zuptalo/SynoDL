import { describe, expect, it } from 'vitest';
import { decideUpdate } from './app-update';

describe('decideUpdate', () => {
  it('prompts the first time a waiting update appears', () => {
    expect(decideUpdate({ waiting: true, runningVersion: '0.0.4', pendingVersion: null })).toEqual({
      clearFlag: false,
      autoApply: false,
      prompt: true,
    });
  });

  it('auto-applies an interrupted update (pending target not yet reached)', () => {
    // User pressed OK last session (pending '0.0.5') but the reload was cut off;
    // still on 0.0.4 with a waiting worker → finish it without asking.
    expect(decideUpdate({ waiting: true, runningVersion: '0.0.4', pendingVersion: '0.0.5' })).toEqual({
      clearFlag: false,
      autoApply: true,
      prompt: false,
    });
  });

  it('clears the flag once the target version is running', () => {
    expect(decideUpdate({ waiting: false, runningVersion: '0.0.5', pendingVersion: '0.0.5' })).toEqual({
      clearFlag: true,
      autoApply: false,
      prompt: false,
    });
  });

  it('clears the flag even if a further update is already waiting', () => {
    // We reached 0.0.5 (clear that flag); a newer 0.0.6 waiting will re-prompt
    // on the next evaluation once the stale flag is gone.
    expect(decideUpdate({ waiting: true, runningVersion: '0.0.5', pendingVersion: '0.0.5' })).toEqual({
      clearFlag: true,
      autoApply: false,
      prompt: false,
    });
  });

  it('does nothing when there is no waiting update and no pending flag', () => {
    expect(decideUpdate({ waiting: false, runningVersion: '0.0.4', pendingVersion: null })).toEqual({
      clearFlag: false,
      autoApply: false,
      prompt: false,
    });
  });
});
