import { describe, expect, it } from 'vitest';

import { safeReturnTo } from '../../src/router';

describe('safeReturnTo', () => {
  it('keeps an internal path with its query and hash', () => {
    expect(safeReturnTo('/video/7?from=cat#comments')).toBe('/video/7?from=cat#comments');
  });

  it.each(['//evil.test/path', '/\\evil.test/path', 'https://evil.test/path', '/login?returnTo=/me']) (
    'rejects unsafe or recursive login destination %s',
    (value) => {
      expect(safeReturnTo(value, '/fallback')).toBe('/fallback');
    },
  );
});
