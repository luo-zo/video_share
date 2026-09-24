import type { RequestOptions } from './client';

export type ReportTarget = 'video' | 'comment' | 'user';
export type ReportStatus = 'open' | 'investigating' | 'resolved' | 'rejected';

export interface ReportItem {
  readonly id: string | number;
  readonly reporter_id: string | number;
  readonly target_type: ReportTarget;
  readonly target_id: string | number;
  readonly reason_code: string;
  readonly detail: string;
  readonly status: ReportStatus;
  readonly assigned_to?: string | number;
  readonly resolution_reason?: string;
  readonly action_id?: string | number;
  readonly created_at: string;
  readonly closed_at?: string;
}

export interface ModerationActionItem {
  readonly id: string | number;
  readonly actor_id: string | number;
  readonly report_id?: string | number;
  readonly target_type: ReportTarget;
  readonly target_id: string | number;
  readonly action: string;
  readonly reason: string;
  readonly before_state: string;
  readonly after_state: string;
  readonly request_id: string;
  readonly created_at: string;
}

export interface ModerationPage<T> {
  readonly items: readonly T[];
  readonly page: number;
  readonly page_size: number;
  readonly total: number;
}

export interface ModerationRequester {
  requestWithSession(path: string, options?: RequestOptions): Promise<unknown>;
}

function id(value: unknown, label: string): string {
  const raw = String(value);
  if (!/^\d+$/.test(raw) || Number(raw) < 1) throw new Error(`${label}无效。`);
  return encodeURIComponent(raw);
}

function requestID(): string {
  const cryptoObject = globalThis.crypto as Crypto & { randomUUID?: () => string } | undefined;
  return cryptoObject?.randomUUID?.() ?? `moderation-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function page<T>(value: unknown): ModerationPage<T> {
  if (!value || typeof value !== 'object') throw new Error('治理列表响应无效。');
  const source = value as Record<string, unknown>;
  if (!Array.isArray(source.items) || !Number.isInteger(source.page) || !Number.isInteger(source.page_size) || !Number.isInteger(source.total)) throw new Error('治理列表响应无效。');
  return { items: source.items as T[], page: source.page as number, page_size: source.page_size as number, total: source.total as number };
}

const pathPage = (path: string, pageNumber = 1, pageSize = 20): string => `${path}?page=${Math.max(1, Number(pageNumber) || 1)}&page_size=${Math.min(50, Math.max(1, Number(pageSize) || 20))}`;

export function createModerationClient(requester: ModerationRequester) {
  const write = (path: string, method: string, body: Record<string, unknown>) => requester.requestWithSession(path, { method, body, withCredentials: true, csrf: true, headers: { 'Idempotency-Key': String(body.request_id ?? requestID()) } });
  return {
    async createReport(input: { targetType: ReportTarget; targetID: string | number; reasonCode: string; detail?: string; requestID?: string }): Promise<ReportItem> {
      const body = { target_type: input.targetType, target_id: Number(id(input.targetID, '目标编号')), reason_code: input.reasonCode, detail: input.detail ?? '', request_id: input.requestID ?? requestID() };
      return await write('/reports', 'POST', body) as ReportItem;
    },
    async listMine(options: { page?: number; pageSize?: number; signal?: AbortSignal } = {}): Promise<ModerationPage<ReportItem>> {
      return page<ReportItem>(await requester.requestWithSession(pathPage('/users/me/reports', options.page, options.pageSize), { signal: options.signal }));
    },
    async listAdmin(options: { page?: number; pageSize?: number; signal?: AbortSignal } = {}): Promise<ModerationPage<ReportItem>> {
      return page<ReportItem>(await requester.requestWithSession(pathPage('/admin/reports', options.page, options.pageSize), { signal: options.signal }));
    },
    async assign(reportID: string | number, requestIDValue = requestID()): Promise<ReportItem> {
      return await write(`/admin/reports/${id(reportID, '举报编号')}/assign`, 'PATCH', { request_id: requestIDValue }) as ReportItem;
    },
    async decide(reportID: string | number, input: { status: 'resolved' | 'rejected'; resolutionReason: string; action?: string; reason?: string; requestID?: string }): Promise<ReportItem> {
      return await write(`/admin/reports/${id(reportID, '举报编号')}`, 'PATCH', { status: input.status, resolution_reason: input.resolutionReason, action: input.action ?? '', reason: input.reason ?? '', request_id: input.requestID ?? requestID() }) as ReportItem;
    },
    async moderate(targetType: ReportTarget, targetID: string | number, action: string, reason: string, reportID?: string | number, requestIDValue = requestID()): Promise<ModerationActionItem> {
      return await write(`/admin/${targetType}s/${id(targetID, '目标编号')}/${encodeURIComponent(action)}`, 'POST', { reason, report_id: reportID ? Number(id(reportID, '举报编号')) : 0, request_id: requestIDValue }) as ModerationActionItem;
    },
    async listActions(options: { page?: number; pageSize?: number; signal?: AbortSignal } = {}): Promise<ModerationPage<ModerationActionItem>> {
      return page<ModerationActionItem>(await requester.requestWithSession(pathPage('/admin/moderation-actions', options.page, options.pageSize), { signal: options.signal }));
    },
  };
}
