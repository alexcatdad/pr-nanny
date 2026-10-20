import { describe, expect, it } from 'vitest';
import { relativeAge, statusLabel } from './format';

describe('display formatting', () => {
  it('uses short friendly ages and clamps future timestamps', () => {
    const now = Date.parse('2026-10-20T05:00:00Z');
    expect(relativeAge('2026-10-20T02:00:00Z', now)).toBe('3h');
    expect(relativeAge('2026-10-18T05:00:00Z', now)).toBe('2d');
    expect(relativeAge('2026-10-20T05:01:00Z', now)).toBe('now');
  });
  it('keeps unknown CI informational', () => {
    expect(statusLabel({ approved: false, ci: 'some_new_api_state' }).ci).toBe('Not available');
  });
});
