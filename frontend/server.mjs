import http from 'node:http';
import https from 'node:https';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const directory = path.dirname(fileURLToPath(import.meta.url));
export const DEFAULT_API_TARGET = 'http://127.0.0.1:8081';
export const DEFAULT_STORAGE_ORIGIN = 'http://127.0.0.1:9000';
const staticFiles = new Map([
  // The standalone migration server deliberately serves the retained legacy shell. Vite owns
  // index.html during the Vue migration, so serving it here would point at an unallowlisted .ts file.
  ['/', ['legacy.html', 'text/html; charset=utf-8']],
  ['/index.html', ['legacy.html', 'text/html; charset=utf-8']],
  ['/src/styles.css', ['src/styles.css', 'text/css; charset=utf-8']],
  ['/src/main.js', ['src/main.js', 'text/javascript; charset=utf-8']],
  ['/src/auth.js', ['src/auth.js', 'text/javascript; charset=utf-8']],
  ['/src/cat.js', ['src/cat.js', 'text/javascript; charset=utf-8']],
  ['/src/video.js', ['src/video.js', 'text/javascript; charset=utf-8']],
  ['/src/community.js', ['src/community.js', 'text/javascript; charset=utf-8']],
  ['/src/view-kit.js', ['src/view-kit.js', 'text/javascript; charset=utf-8']],
  ['/src/discover-view.js', ['src/discover-view.js', 'text/javascript; charset=utf-8']],
  ['/src/detail-view.js', ['src/detail-view.js', 'text/javascript; charset=utf-8']],
  ['/src/profile-view.js', ['src/profile-view.js', 'text/javascript; charset=utf-8']],
  ['/src/player.js', ['src/player.js', 'text/javascript; charset=utf-8']],
  ['/vendor/hls.mjs', ['node_modules/hls.js/dist/hls.mjs', 'text/javascript; charset=utf-8']],
]);
const fixedApiRoutes = new Map([
  ['/healthz', ['GET']],
  ['/readyz', ['GET']],
  ['/api/v1/users/me', ['GET']],
  ['/api/v1/users/me/videos', ['GET']],
  ['/api/v1/users/me/favorites', ['GET']],
  ['/api/v1/users/me/history', ['GET']],
  ['/api/v1/users/me/follows', ['GET']],
  ['/api/v1/auth/login', ['POST']],
  ['/api/v1/auth/register', ['POST']],
  ['/api/v1/videos', ['GET', 'POST']],
]);
// 这些方法可以携带请求体；没有请求体的删除调用仍然直接转发。
const WRITE_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

export function methodsForAPIPath(route) {
  const fixed = fixedApiRoutes.get(route);
  if (fixed) return fixed;
  if (/^\/api\/v1\/videos\/[1-9]\d*$/.test(route)) return ['GET'];
  if (/^\/api\/v1\/users\/me\/videos\/[1-9]\d*$/.test(route)) return ['GET', 'PATCH', 'DELETE'];
  if (/^\/api\/v1\/users\/[1-9]\d*\/follow$/.test(route)) return ['PUT', 'DELETE'];
  if (/^\/api\/v1\/videos\/[1-9]\d*\/(?:like|favorite)$/.test(route)) return ['PUT', 'DELETE'];
  if (/^\/api\/v1\/videos\/[1-9]\d*\/comments$/.test(route)) return ['GET', 'POST'];
  if (/^\/api\/v1\/videos\/[1-9]\d*\/watch$/.test(route)) return ['POST'];
  if (/^\/api\/v1\/comments\/[1-9]\d*$/.test(route)) return ['DELETE'];
  if (/^\/api\/v1\/videos\/[1-9]\d*\/cover$/.test(route)) return ['GET'];
  const hls = route.match(/^\/api\/v1\/videos\/[1-9]\d*\/hls\/(.+)$/);
  if (hls && hls[1].split('/').every((part) => part !== '.' && part !== '..' && /^[A-Za-z0-9._-]+$/.test(part))) return ['GET'];
  if (/^\/api\/v1\/videos\/[1-9]\d*\/complete$/.test(route)) return ['POST'];
  return null;
}

function hasBody(request) {
  if (request.headers['transfer-encoding'] !== undefined) return true;
  const length = Number(request.headers['content-length']);
  return Number.isFinite(length) && length > 0;
}

class RequestError extends Error {
  constructor(status, code, message) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export function securityHeaders(response, storageOrigin) {
  response.setHeader('X-Content-Type-Options', 'nosniff');
  response.setHeader('Referrer-Policy', 'no-referrer');
  response.setHeader('Cross-Origin-Resource-Policy', 'same-origin');
  response.setHeader('X-Frame-Options', 'DENY');
  response.setHeader('Content-Security-Policy', [
    "default-src 'self'",
    "script-src 'self'",
    "style-src 'self'",
    `img-src 'self' data: ${storageOrigin}`,
    `connect-src 'self' ${storageOrigin}`,
    `media-src 'self' blob: ${storageOrigin}`,
    "object-src 'none'",
    "base-uri 'none'",
    "frame-ancestors 'none'",
    "form-action 'self'",
  ].join('; '));
}

function sendError(response, status, code, message) {
  if (response.destroyed || response.writableEnded) return;
  response.writeHead(status, {
    'Content-Type': 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
  });
  response.end(JSON.stringify({ error: { code, message } }));
}

export function isLocalOrigin(request) {
  try {
    const frontend = new URL(`http://${request.headers.host}`);
    const port = Number(frontend.port || 80);
    if (!['localhost', '127.0.0.1', '[::1]'].includes(frontend.hostname)
      || port !== request.socket.localPort
      || frontend.username || frontend.password
      || frontend.pathname !== '/' || frontend.search || frontend.hash) return false;
    if (request.headers.origin === undefined) return true;
    return request.headers.origin === frontend.origin;
  } catch {
    return false;
  }
}

function readBody(request, maxBytes, timeoutMs) {
  return new Promise((resolve, reject) => {
    let size = 0;
    const chunks = [];
    const timer = setTimeout(() => finish(new RequestError(
      408, 'REQUEST_TIMEOUT', 'Request body timed out.',
    )), timeoutMs);
    function cleanup() {
      clearTimeout(timer);
      request.off('data', onData);
      request.off('end', onEnd);
      request.off('error', onError);
      request.off('aborted', onAborted);
    }
    function finish(error) {
      cleanup();
      if (error) {
        request.resume();
        reject(error);
      } else {
        resolve(Buffer.concat(chunks));
      }
    }
    function onData(chunk) {
      size += chunk.length;
      if (size > maxBytes) {
        finish(new RequestError(413, 'BODY_TOO_LARGE', 'Request body exceeds 32 KiB.'));
      } else {
        chunks.push(chunk);
      }
    }
    function onEnd() { finish(); }
    function onError() { finish(new RequestError(400, 'INVALID_REQUEST', 'Could not read request.')); }
    function onAborted() { finish(new RequestError(400, 'INVALID_REQUEST', 'Request was interrupted.')); }
    request.on('data', onData);
    request.once('end', onEnd);
    request.once('error', onError);
    request.once('aborted', onAborted);
  });
}

function callUpstream(target, request, route, body, timeoutMs) {
  return new Promise((resolve, reject) => {
    const transport = target.protocol === 'https:' ? https : http;
    const headers = { Accept: 'application/json' };
    if (request.headers.authorization) headers.Authorization = request.headers.authorization;
    if (body !== undefined) {
      headers['Content-Type'] = request.headers['content-type'];
      headers['Content-Length'] = body.length;
    }
    let settled = false;
    let timer;
    const outgoing = transport.request(new URL(route, target), {
      method: request.method,
      headers,
    }, (incoming) => {
      const chunks = [];
      let size = 0;
      incoming.on('data', (chunk) => {
        size += chunk.length;
        if (size > 1024 * 1024) {
          finish(new RequestError(502, 'UPSTREAM_ERROR', 'API response is too large.'));
          incoming.destroy();
          outgoing.destroy();
        } else {
          chunks.push(chunk);
        }
      });
      incoming.once('end', () => finish(null, {
        status: incoming.statusCode,
        contentType: incoming.headers['content-type'] || 'application/json; charset=utf-8',
        location: incoming.headers.location,
        body: Buffer.concat(chunks),
      }));
      incoming.once('error', () => finish(new RequestError(
        502, 'UPSTREAM_UNAVAILABLE', 'The Video Share API connection was interrupted.',
      )));
    });
    function finish(error, result) {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (error) reject(error);
      else resolve(result);
    }
    outgoing.once('error', () => finish(new RequestError(
      502, 'UPSTREAM_UNAVAILABLE', 'Cannot connect to the Video Share API. Please check that the backend is running.',
    )));
    timer = setTimeout(() => {
      finish(new RequestError(504, 'UPSTREAM_TIMEOUT', 'The Video Share API did not respond in time.'));
      outgoing.destroy();
    }, timeoutMs);
    outgoing.end(body);
  });
}

export function resolveOrigins(apiTarget, storageOrigin) {
  const target = new URL(apiTarget);
  if (!['http:', 'https:'].includes(target.protocol) || target.username || target.password
    || target.pathname !== '/' || target.search || target.hash) {
    throw new Error('API_TARGET must be an HTTP(S) origin without credentials, a path, or a query.');
  }
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(target.hostname);
  if (target.protocol !== 'https:' && !loopback) {
    throw new Error('API_TARGET must use HTTPS unless it is a loopback origin.');
  }
  const storage = new URL(storageOrigin);
  const storageLoopback = ['localhost', '127.0.0.1', '[::1]'].includes(storage.hostname);
  if (!['http:', 'https:'].includes(storage.protocol) || storage.username || storage.password
      || storage.pathname !== '/' || storage.search || storage.hash
      || (storage.protocol !== 'https:' && !storageLoopback)) {
    throw new Error('STORAGE_ORIGIN must be an HTTPS origin, or a loopback HTTP origin.');
  }
  return { target, storage };
}

// 同一份安全与转发逻辑同时驱动旧的独立前端服务和 Vite 开发服务器，避免两套代理行为漂移。
// 作为中间件使用时，非 API 路径交给 next()，由调用方决定静态资源或 404 的处理。
export function createApiMiddleware({
  apiTarget = process.env.API_TARGET || DEFAULT_API_TARGET,
  storageOrigin = process.env.STORAGE_ORIGIN || DEFAULT_STORAGE_ORIGIN,
  timeoutMs = 12_000,
  maxBodyBytes = 32 * 1024,
} = {}) {
  const { target, storage } = resolveOrigins(apiTarget, storageOrigin);

  return async function apiMiddleware(request, response, next) {
    securityHeaders(response, storage.origin);
    // Match the raw path: encoded traversal and alternate spellings never map to disk.
    const route = (request.url || '').split('?')[0];
    const apiMethods = methodsForAPIPath(route);
    try {
      if (!apiMethods) {
        next();
        return;
      }
      if (!apiMethods.includes(request.method)) {
        const allowed = apiMethods.join(', ');
        response.setHeader('Allow', allowed);
        throw new RequestError(405, 'METHOD_NOT_ALLOWED', `This resource supports ${allowed}.`);
      }
      let body;
      if (WRITE_METHODS.has(request.method)) {
        if (!isLocalOrigin(request)) {
          throw new RequestError(403, 'ORIGIN_REJECTED', 'Cross-origin requests are not allowed.');
        }
        if (hasBody(request)) {
          if (!/^application\/json(?:\s*;|\s*$)/i.test(request.headers['content-type'] || '')) {
            throw new RequestError(415, 'JSON_REQUIRED', 'Content-Type must be application/json.');
          }
          body = await readBody(request, maxBodyBytes, timeoutMs);
        }
      }
      const upstream = await callUpstream(target, request, request.url, body, timeoutMs);
      const headers = {
        'Content-Type': upstream.contentType,
        'Content-Length': upstream.body.length,
        'Cache-Control': 'no-store',
      };
      if (upstream.status >= 300 && upstream.status < 400 && upstream.location) {
        try {
          const location = new URL(upstream.location, target);
          if (!location.username && !location.password && location.origin === storage.origin) {
            headers.Location = location.href;
          }
        } catch {
          // An invalid upstream redirect is intentionally not exposed to the browser.
        }
      }
      response.writeHead(upstream.status, headers);
      response.end(upstream.body);
    } catch (error) {
      request.resume();
      if (!request.complete) response.setHeader('Connection', 'close');
      if (error instanceof RequestError) sendError(response, error.status, error.code, error.message);
      else if (error.code === 'ENOENT') sendError(response, 404, 'NOT_FOUND', 'Resource not found.');
      else sendError(response, 500, 'INTERNAL_ERROR', 'Unable to serve this request.');
    }
  };
}

export function createFrontendServer({
  rootDir = directory,
  apiTarget = process.env.API_TARGET || DEFAULT_API_TARGET,
  storageOrigin = process.env.STORAGE_ORIGIN || DEFAULT_STORAGE_ORIGIN,
  timeoutMs = 12_000,
  maxBodyBytes = 32 * 1024,
} = {}) {
  const { storage } = resolveOrigins(apiTarget, storageOrigin);
  const apiMiddleware = createApiMiddleware({ apiTarget, storageOrigin, timeoutMs, maxBodyBytes });

  async function serveStatic(request, response) {
    securityHeaders(response, storage.origin);
    // Match the raw path: encoded traversal and alternate spellings never map to disk.
    const route = (request.url || '').split('?')[0];
    const staticFile = staticFiles.get(route);
    try {
      if (!staticFile) throw new RequestError(404, 'NOT_FOUND', 'Resource not found.');
      if (!['GET', 'HEAD'].includes(request.method)) {
        response.setHeader('Allow', 'GET, HEAD');
        throw new RequestError(405, 'METHOD_NOT_ALLOWED', 'This resource supports GET and HEAD.');
      }
      const [filename, contentType] = staticFile;
      const body = await readFile(path.join(rootDir, filename));
      response.writeHead(200, {
        'Content-Type': contentType,
        'Content-Length': body.length,
        'Cache-Control': 'no-cache',
      });
      response.end(request.method === 'HEAD' ? undefined : body);
    } catch (error) {
      request.resume();
      if (!request.complete) response.setHeader('Connection', 'close');
      if (error instanceof RequestError) sendError(response, error.status, error.code, error.message);
      else if (error.code === 'ENOENT') sendError(response, 404, 'NOT_FOUND', 'Resource not found.');
      else sendError(response, 500, 'INTERNAL_ERROR', 'Unable to serve this request.');
    }
  }

  return http.createServer((request, response) => {
    const route = (request.url || '').split('?')[0];
    if (staticFiles.has(route) || !methodsForAPIPath(route)) {
      return serveStatic(request, response);
    }
    return apiMiddleware(request, response, () => {
      sendError(response, 404, 'NOT_FOUND', 'Resource not found.');
    });
  });
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const port = Number(process.env.PORT || 5173);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error('PORT must be an integer between 1 and 65535.');
  }
  const server = createFrontendServer();
  server.on('error', (error) => {
    console.error(`Frontend server failed: ${error.code || 'UNKNOWN_ERROR'}`);
    process.exitCode = 1;
  });
  server.listen(port, '127.0.0.1', () => {
    console.log(`Video Share frontend: http://127.0.0.1:${port}`);
  });
}
