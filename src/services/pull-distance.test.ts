import { describe, expect, it } from 'vitest';
import { pullDistance } from './pull-distance';

describe('pullDistance', () => {
  it('reads the downward shift out of a 2D matrix', () => {
    expect(pullDistance('matrix(1, 0, 0, 1, 0, 64)')).toBe(64);
  });

  it('reads it out of a 3D matrix, where it is the fourteenth value', () => {
    expect(pullDistance('matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 92, 0, 1)')).toBe(92);
  });

  it('is zero at rest', () => {
    expect(pullDistance('none')).toBe(0);
    expect(pullDistance('')).toBe(0);
    expect(pullDistance(null)).toBe(0);
    expect(pullDistance(undefined)).toBe(0);
  });

  it('treats an upward shift as no pull, not a negative one', () => {
    // Drawn from a negative distance the droplet would hang above its origin,
    // pointing the wrong way.
    expect(pullDistance('matrix(1, 0, 0, 1, 0, -30)')).toBe(0);
  });

  it('ignores a horizontal shift', () => {
    expect(pullDistance('matrix(1, 0, 0, 1, 120, 0)')).toBe(0);
  });

  it('is zero for anything it cannot read', () => {
    expect(pullDistance('translateY(40px)')).toBe(0);
    expect(pullDistance('matrix(1, 0, 0, 1, 0, banana)')).toBe(0);
    expect(pullDistance('matrix(1, 0, 0, 1)')).toBe(0);
  });
});
