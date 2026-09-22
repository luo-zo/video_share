import type { IncomingMessage, Server, ServerResponse } from 'node:http';

export declare const DEFAULT_API_TARGET: string;
export declare const DEFAULT_STORAGE_ORIGIN: string;

export interface OriginPair {
  target: URL;
  storage: URL;
}

export interface ApiMiddlewareOptions {
  apiTarget?: string;
  storageOrigin?: string;
  timeoutMs?: number;
  maxBodyBytes?: number;
}

export type ConnectNext = (error?: unknown) => void;

export type ApiMiddleware = (
  request: IncomingMessage,
  response: ServerResponse,
  next: ConnectNext,
) => Promise<void>;

export declare function methodsForAPIPath(route: string): string[] | null;
export declare function securityHeaders(response: ServerResponse, storageOrigin: string): void;
export declare function isLocalOrigin(request: IncomingMessage): boolean;
export declare function resolveOrigins(apiTarget: string, storageOrigin: string): OriginPair;
export declare function createApiMiddleware(options?: ApiMiddlewareOptions): ApiMiddleware;
export declare function createFrontendServer(
  options?: ApiMiddlewareOptions & { rootDir?: string },
): Server;
