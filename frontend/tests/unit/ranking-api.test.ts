import { describe, expect, it, vi } from 'vitest';

import { createRankingClient } from '../../src/api/ranking';

describe('ranking API', () => {
  it('requests a validated day/week snapshot with stable pagination', async () => {
    const requestPublic = vi.fn(async (_path: string) => ({
      window: 'week', generated_at: '2026-09-23T04:00:00Z', page: 2, page_size: 20, total: 21,
      items: [{ id: 9, video_id: 9, rank: 21, score: 100, title: '猫' }],
    }));
    const authClient = { requestPublic, requestWithSession: vi.fn(async () => ({})), requestWithOptionalSession: vi.fn(async () => ({})) };
    const client = createRankingClient({ authClient });
    await expect(client.list({ window: 'week', page: 2, pageSize: 20 })).resolves.toMatchObject({ window: 'week', total: 21 });
    expect(requestPublic.mock.calls[0]?.[0]).toBe('/videos/ranking?window=week&page=2&page_size=20');
  });

  it('falls back to day and bounds page size for invalid input', async () => {
    const requestPublic = vi.fn(async (_path: string) => ({ window: 'day', generated_at: '', page: 1, page_size: 50, total: 0, items: [] }));
    const authClient = { requestPublic, requestWithSession: vi.fn(async () => ({})), requestWithOptionalSession: vi.fn(async () => ({})) };
    const client = createRankingClient({ authClient });
    await client.list({ window: 'month' as 'day', page: 0, pageSize: 100 });
    expect(requestPublic.mock.calls[0]?.[0]).toBe('/videos/ranking?window=day&page=1&page_size=50');
  });
});
