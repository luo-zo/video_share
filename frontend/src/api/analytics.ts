import type { RequestOptions } from './client';
import type { SessionRequester } from './video';

export interface WatchStart {
  readonly session_id: string;
  readonly video_id: string | number;
  readonly duration_ms: number;
  readonly resume_position_ms: number;
  readonly expires_at: string;
}

export interface HeartbeatInput {
  readonly seq: number;
  readonly position_ms: number;
  readonly watched_delta_ms: number;
  readonly ended?: boolean;
}

export interface HeartbeatResult {
  readonly session_id: string;
  readonly seq: number;
  readonly position_ms: number;
  readonly accepted_delta_ms: number;
  readonly session_credited_ms: number;
  readonly effective_watch_ms: number;
  readonly qualified: boolean;
  readonly completed: boolean;
}

export interface CreatorSummary {
  readonly days: number;
  readonly metric_version: string;
  readonly as_of: string;
  readonly current: {
    readonly video_count: number;
    readonly view_count: number;
    readonly like_count: number;
    readonly favorite_count: number;
    readonly comment_count: number;
    readonly follower_count: number;
  };
  readonly trend: readonly {
    readonly date: string;
    readonly effective_views: number;
    readonly watch_time_ms: number;
    readonly completions: number;
    readonly net_likes: number;
    readonly net_favorites: number;
    readonly net_comments: number;
    readonly net_followers: number;
  }[];
  readonly top_videos: readonly {
    readonly video_id: string | number;
    readonly title: string;
    readonly effective_views: number;
    readonly watch_time_ms: number;
    readonly completions: number;
  }[];
}

function positiveID(value: unknown): string {
  if (!/^\d+$/.test(String(value)) || Number(value) < 1) throw new Error('INVALID_PARAMETER');
  return encodeURIComponent(String(value));
}

function startFrom(data: unknown): WatchStart {
  if (!data || typeof data !== 'object') throw new Error('INVALID_RESPONSE');
  const row = data as Record<string, unknown>;
  if (typeof row.session_id !== 'string' || typeof row.video_id !== 'string' && typeof row.video_id !== 'number') throw new Error('INVALID_RESPONSE');
  return data as WatchStart;
}

function heartbeatFrom(data: unknown): HeartbeatResult {
  if (!data || typeof data !== 'object') throw new Error('INVALID_RESPONSE');
  const row = data as Record<string, unknown>;
  if (typeof row.session_id !== 'string' || !Number.isInteger(row.seq) || !Number.isInteger(row.accepted_delta_ms)) throw new Error('INVALID_RESPONSE');
  return data as HeartbeatResult;
}

function summaryFrom(data: unknown): CreatorSummary {
  if (!data || typeof data !== 'object') throw new Error('INVALID_RESPONSE');
  const row = data as Record<string, unknown>;
  if (!Number.isInteger(row.days) || !Array.isArray(row.trend) || !Array.isArray(row.top_videos) || !row.current || typeof row.current !== 'object') throw new Error('INVALID_RESPONSE');
  return data as CreatorSummary;
}

export interface AnalyticsClient {
  startWatchSession(videoID: unknown, options?: RequestOptions): Promise<WatchStart>;
  heartbeat(sessionID: string, input: HeartbeatInput, options?: RequestOptions): Promise<HeartbeatResult>;
  creatorSummary(days?: 7 | 30, options?: RequestOptions): Promise<CreatorSummary>;
}

export function createAnalyticsClient(authClient: SessionRequester): AnalyticsClient {
  return {
    async startWatchSession(videoID, options = {}): Promise<WatchStart> {
      return startFrom(await authClient.requestWithSession(`/videos/${positiveID(videoID)}/watch-sessions`, { method: 'POST', ...options }));
    },
    async heartbeat(sessionID, input, options = {}): Promise<HeartbeatResult> {
      if (!sessionID.trim() || !Number.isInteger(input.seq) || input.seq < 1 || !Number.isInteger(input.position_ms) || input.position_ms < 0 || !Number.isInteger(input.watched_delta_ms) || input.watched_delta_ms < 0 || input.watched_delta_ms > 15_000) throw new Error('INVALID_PARAMETER');
      return heartbeatFrom(await authClient.requestWithSession(`/watch-sessions/${encodeURIComponent(sessionID)}/heartbeat`, { method: 'POST', body: input, ...options }));
    },
    async creatorSummary(days = 7, options = {}): Promise<CreatorSummary> {
      if (days !== 7 && days !== 30) throw new Error('INVALID_PARAMETER');
      return summaryFrom(await authClient.requestWithSession(`/users/me/creator/summary?days=${days}`, options));
    },
  };
}
