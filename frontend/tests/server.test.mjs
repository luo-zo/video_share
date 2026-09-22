import assert from 'node:assert/strict';
import http from 'node:http';
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { after, before, test } from 'node:test';
import { createFrontendServer, DEFAULT_API_TARGET, DEFAULT_STORAGE_ORIGIN } from '../server.mjs';

test('targets the backend port configured by this project by default', () => {
  assert.equal(DEFAULT_API_TARGET, 'http://127.0.0.1:8081');
  assert.equal(DEFAULT_STORAGE_ORIGIN, 'http://127.0.0.1:9000');
});

test('application document loads the JavaScript module entrypoint', async () => {
  const html = await readFile(new URL('../index.html', import.meta.url), 'utf8');
  assert.match(html, /<script\s+type="module"\s+src="\/src\/main\.ts"><\/script>/);
});

test('legacy server homepage loads its allowlisted runnable entrypoint', async (t) => {
  const origin = await listen(createFrontendServer(), t);
  const homepage = await rawRequest(origin, '/');
  assert.equal(homepage.status, 200);
  assert.match(homepage.body, /id="auth-form"/);
  assert.match(homepage.body, /<script\s+type="module"\s+src="\/src\/main\.js"><\/script>/);
  const entry = await rawRequest(origin, '/src/main.js');
  assert.equal(entry.status, 200);
  assert.match(entry.headers['content-type'], /javascript/);
});

let rootDir;
before(async () => {
  rootDir = await mkdtemp(path.join(os.tmpdir(), 'video-share-frontend-'));
  await mkdir(path.join(rootDir, 'src'));
  await mkdir(path.join(rootDir, 'node_modules', 'hls.js', 'dist'), { recursive: true });
  await writeFile(path.join(rootDir, 'legacy.html'), '<!doctype html><title>Video Share</title>');
  await writeFile(path.join(rootDir, 'src', 'main.js'), 'export const ready = true;');
  await writeFile(path.join(rootDir, 'node_modules', 'hls.js', 'dist', 'hls.mjs'), 'export default class Hls {}');
  await writeFile(path.join(rootDir, 'secret.txt'), 'must never be served');
});
after(async () => rm(rootDir, { recursive: true, force: true }));

async function listen(server, t) {
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  t.after(() => new Promise((resolve, reject) => {
    server.close((error) => error ? reject(error) : resolve());
    server.closeAllConnections();
  }));
  return `http://127.0.0.1:${server.address().port}`;
}

function rawRequest(origin, route, options = {}) {
  // Node 把 DELETE 当作无请求体的方法，不会自动补 Content-Length；若不显式声明，
  // 请求体会以未分帧的裸字节发出，被服务端解析器直接以 400 拒绝。
  const headers = options.body === undefined
    ? options.headers
    : { ...options.headers, 'Content-Length': Buffer.byteLength(options.body) };
  return new Promise((resolve, reject) => {
    const outgoing = http.request(origin, { path: route, ...options, headers }, (incoming) => {
      const chunks = [];
      incoming.on('data', (chunk) => chunks.push(chunk));
      incoming.on('end', () => resolve({
        status: incoming.statusCode,
        headers: incoming.headers,
        body: Buffer.concat(chunks).toString(),
      }));
    });
    outgoing.on('error', reject);
    outgoing.end(options.body);
  });
}

test('serves only allowlisted static files, including HEAD and cache-busting queries', async (t) => {
  const origin = await listen(createFrontendServer({ rootDir }), t);
  const response = await rawRequest(origin, '/');
  assert.equal(response.status, 200);
  assert.match(response.body, /Video Share/);
  assert.match(response.headers['content-type'], /text\/html/);
  assert.match(response.headers['content-security-policy'], /script-src 'self'/);
  assert.match(response.headers['content-security-policy'], /media-src 'self' blob: http:\/\/127\.0\.0\.1:9000/);
  assert.match(response.headers['content-security-policy'], /img-src 'self' data: http:\/\/127\.0\.0\.1:9000/);
  assert.equal(response.headers['x-content-type-options'], 'nosniff');
  const script = await rawRequest(origin, '/src/main.js?v=1');
  assert.equal(script.status, 200);
  assert.match(script.headers['content-type'], /javascript/);
  const playerLibrary = await rawRequest(origin, '/vendor/hls.mjs');
  assert.equal(playerLibrary.status, 200);
  assert.match(playerLibrary.headers['content-type'], /javascript/);
  const head = await rawRequest(origin, '/', { method: 'HEAD' });
  assert.equal(head.status, 200);
  assert.equal(head.body, '');
  for (const route of ['/secret.txt', '/../secret.txt', '/src/../secret.txt', '/src/%2e%2e/secret.txt', '/%2e%2e/secret.txt', '//index.html', '/server.mjs']) {
    assert.equal((await rawRequest(origin, route)).status, 404, route);
  }
});

test('rejects unknown API routes and wrong methods without contacting upstream', async (t) => {
  let upstreamCalls = 0;
  const apiTarget = await listen(http.createServer((request, response) => {
    upstreamCalls += 1;
    response.end('{}');
  }), t);
  const origin = await listen(createFrontendServer({ rootDir, apiTarget }), t);
  assert.equal((await rawRequest(origin, '/api/v1/videos/0')).status, 404);
  const wrongMethod = await rawRequest(origin, '/api/v1/auth/login');
  assert.equal(wrongMethod.status, 405);
  assert.equal(wrongMethod.headers.allow, 'POST');
  assert.equal((await rawRequest(origin, '/', { method: 'POST' })).status, 405);
  assert.equal(upstreamCalls, 0);
});

test('proxies allowlisted video routes, methods, auth and pagination queries', async (t) => {
  const seen = [];
  const apiTarget = await listen(http.createServer((request, response) => {
    seen.push({ method: request.method, url: request.url, auth: request.headers.authorization });
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end('{"data":{}}');
  }), t);
  const origin = await listen(createFrontendServer({ rootDir, apiTarget }), t);

  assert.equal((await rawRequest(origin, '/api/v1/videos?page=2&page_size=12')).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/videos/7')).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/users/me/videos?page=1', {
    headers: { Authorization: 'Bearer test-token' },
  })).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/users/me/videos/7', {
    headers: { Authorization: 'Bearer test-token' },
  })).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/videos/7/hls/master.m3u8')).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/videos/7/cover')).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/videos', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: origin, Authorization: 'Bearer test-token' },
    body: '{"title":"hello"}',
  })).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/videos/7/complete', {
    method: 'POST', headers: { 'Content-Type': 'application/json', Origin: origin }, body: '{}',
  })).status, 200);

  assert.deepEqual(seen.map(({ method, url }) => `${method} ${url}`), [
    'GET /api/v1/videos?page=2&page_size=12',
    'GET /api/v1/videos/7',
    'GET /api/v1/users/me/videos?page=1',
    'GET /api/v1/users/me/videos/7',
    'GET /api/v1/videos/7/hls/master.m3u8',
    'GET /api/v1/videos/7/cover',
    'POST /api/v1/videos',
    'POST /api/v1/videos/7/complete',
  ]);
  assert.equal(seen[2].auth, 'Bearer test-token');
  assert.equal(seen[3].auth, 'Bearer test-token');
  assert.equal(seen[6].auth, 'Bearer test-token');
  assert.equal((await rawRequest(origin, '/api/v1/videos/7', { method: 'DELETE' })).status, 405);
  assert.equal((await rawRequest(origin, '/api/v1/videos/not-a-number')).status, 404);
  assert.equal((await rawRequest(origin, '/api/v1/videos/7/hls/../secret.ts')).status, 404);
});

test('proxies community routes with their exact methods and bearer tokens', async (t) => {
  const seen = [];
  const apiTarget = await listen(http.createServer(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    seen.push({
      method: request.method,
      url: request.url,
      auth: request.headers.authorization,
      body: Buffer.concat(chunks).toString(),
    });
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end('{"data":{}}');
  }), t);
  const origin = await listen(createFrontendServer({ rootDir, apiTarget }), t);

  const json = (method, route) => rawRequest(origin, route, {
    method,
    headers: { 'Content-Type': 'application/json', Origin: origin, Authorization: 'Bearer test-token' },
    body: '{}',
  });

  assert.equal((await rawRequest(origin, '/api/v1/videos/9/comments?page=1')).status, 200);
  assert.equal((await json('POST', '/api/v1/videos/9/comments')).status, 200);
  assert.equal((await json('DELETE', '/api/v1/comments/11')).status, 200);
  assert.equal((await json('PUT', '/api/v1/videos/9/like')).status, 200);
  assert.equal((await json('DELETE', '/api/v1/videos/9/like')).status, 200);
  assert.equal((await json('PUT', '/api/v1/videos/9/favorite')).status, 200);
  assert.equal((await json('DELETE', '/api/v1/videos/9/favorite')).status, 200);
  assert.equal((await json('POST', '/api/v1/videos/9/watch')).status, 200);
  assert.equal((await json('PATCH', '/api/v1/users/me/videos/5')).status, 200);
  assert.equal((await json('DELETE', '/api/v1/users/me/videos/5')).status, 200);
  const authed = { headers: { Authorization: 'Bearer test-token' } };
  assert.equal((await rawRequest(origin, '/api/v1/users/me/favorites?page=2', authed)).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/users/me/history', authed)).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/users/me/follows', authed)).status, 200);
  assert.equal((await json('PUT', '/api/v1/users/4/follow')).status, 200);
  assert.equal((await json('DELETE', '/api/v1/users/4/follow')).status, 200);

  assert.deepEqual(seen.map(({ method, url }) => `${method} ${url}`), [
    'GET /api/v1/videos/9/comments?page=1',
    'POST /api/v1/videos/9/comments',
    'DELETE /api/v1/comments/11',
    'PUT /api/v1/videos/9/like',
    'DELETE /api/v1/videos/9/like',
    'PUT /api/v1/videos/9/favorite',
    'DELETE /api/v1/videos/9/favorite',
    'POST /api/v1/videos/9/watch',
    'PATCH /api/v1/users/me/videos/5',
    'DELETE /api/v1/users/me/videos/5',
    'GET /api/v1/users/me/favorites?page=2',
    'GET /api/v1/users/me/history',
    'GET /api/v1/users/me/follows',
    'PUT /api/v1/users/4/follow',
    'DELETE /api/v1/users/4/follow',
  ]);
  for (const request of seen.slice(1)) {
    assert.equal(request.auth, 'Bearer test-token');
    // 写请求必须原样转发请求体，个人列表这类 GET 则不能带上任何请求体。
    assert.equal(request.body, request.method === 'GET' ? '' : '{}');
  }

  assert.equal((await rawRequest(origin, '/api/v1/comments/11')).status, 405);
  assert.equal((await rawRequest(origin, '/api/v1/videos/9/watch')).status, 405);
  assert.equal((await rawRequest(origin, '/api/v1/users/me/follows', { method: 'POST' })).status, 405);
  assert.equal((await rawRequest(origin, '/api/v1/users/4/follow', { method: 'GET' })).status, 405);
  assert.equal((await rawRequest(origin, '/api/v1/comments/0')).status, 404);
});

test('applies origin and JSON content-type checks to every body-carrying method', async (t) => {
  let upstreamCalls = 0;
  const apiTarget = await listen(http.createServer((request, response) => {
    upstreamCalls += 1;
    response.end('{}');
  }), t);
  const origin = await listen(createFrontendServer({ rootDir, apiTarget }), t);

  const crossOrigin = await rawRequest(origin, '/api/v1/videos/9/like', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', Origin: 'https://unrelated.example' },
    body: '{}',
  });
  assert.equal(crossOrigin.status, 403);
  assert.equal(JSON.parse(crossOrigin.body).error.code, 'ORIGIN_REJECTED');

  assert.equal((await rawRequest(origin, '/api/v1/videos/9/like', {
    method: 'PUT', headers: { 'Content-Type': 'text/plain', Origin: origin }, body: '{}',
  })).status, 415);
  assert.equal((await rawRequest(origin, '/api/v1/users/me/videos/5', {
    method: 'PATCH', headers: { 'Content-Type': 'application/json', Origin: origin },
    body: 'x'.repeat(32 * 1024 + 1),
  })).status, 413);
  assert.equal((await rawRequest(origin, '/api/v1/users/4/follow', {
    method: 'DELETE', headers: { 'Content-Type': 'application/json', Origin: origin }, body: '{}',
  })).status, 200);
  assert.equal((await rawRequest(origin, '/api/v1/users/4/follow', {
    method: 'DELETE', headers: { Origin: origin },
  })).status, 200);
  assert.equal(upstreamCalls, 2);
});

test('forwards only MinIO redirects used by HLS segments and covers', async (t) => {
  const apiTarget = await listen(http.createServer((request, response) => {
    const location = request.url.endsWith('/cover')
      ? 'http://127.0.0.1:9000/video-share/cover.jpg?signature=test'
      : 'http://127.0.0.1:9000/video-share/segment.ts?signature=test';
    response.writeHead(307, { Location: location });
    response.end();
  }), t);
  const origin = await listen(createFrontendServer({ rootDir, apiTarget }), t);

  const cover = await rawRequest(origin, '/api/v1/videos/7/cover');
  const segment = await rawRequest(origin, '/api/v1/videos/7/hls/360p/segment-00000.ts');
  assert.equal(cover.status, 307);
  assert.equal(cover.headers.location, 'http://127.0.0.1:9000/video-share/cover.jpg?signature=test');
  assert.equal(segment.status, 307);
  assert.equal(segment.headers.location, 'http://127.0.0.1:9000/video-share/segment.ts?signature=test');
});

test('proxies auth JSON, Bearer credentials and upstream error status/body', async (t) => {
  const seen = [];
  const apiTarget = await listen(http.createServer(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    seen.push({ route: request.url, headers: request.headers, body: Buffer.concat(chunks).toString() });
    response.writeHead(request.url === '/api/v1/auth/login' ? 401 : 200, { 'Content-Type': 'application/json' });
    response.end(request.url === '/api/v1/auth/login'
      ? '{"error":{"code":"INVALID_CREDENTIALS","message":"Incorrect password"}}'
      : '{"data":{"id":7}}');
  }), t);
  const origin = await listen(createFrontendServer({ rootDir, apiTarget }), t);
  const body = JSON.stringify({ username: 'cat', password: 'secret-test-value' });
  const login = await rawRequest(origin, '/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json; charset=utf-8', Origin: origin },
    body,
  });
  assert.equal(login.status, 401);
  assert.equal(JSON.parse(login.body).error.code, 'INVALID_CREDENTIALS');
  assert.equal(seen[0].body, body);
  assert.equal(seen[0].headers['content-type'], 'application/json; charset=utf-8');
  const profile = await rawRequest(origin, '/api/v1/users/me', { headers: { Authorization: 'Bearer test-token' } });
  assert.equal(profile.status, 200);
  assert.deepEqual(JSON.parse(profile.body), { data: { id: 7 } });
  assert.equal(seen[1].headers.authorization, 'Bearer test-token');
  assert.equal(profile.headers['cache-control'], 'no-store');
  assert.equal((await rawRequest(origin, '/api/v1/auth/register', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body,
  })).status, 200);
});

test('rejects cross-origin, non-JSON and oversized POST requests', async (t) => {
  let upstreamCalls = 0;
  const apiTarget = await listen(http.createServer((request, response) => {
    upstreamCalls += 1;
    response.end('{}');
  }), t);
  const origin = await listen(createFrontendServer({ rootDir, apiTarget }), t);
  const rejectedOrigin = await rawRequest(origin, '/api/v1/auth/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json', Origin: 'https://unrelated.example' }, body: '{}',
  });
  assert.equal(rejectedOrigin.status, 403);
  assert.equal(JSON.parse(rejectedOrigin.body).error.code, 'ORIGIN_REJECTED');
  assert.equal((await rawRequest(origin, '/api/v1/auth/register', {
    method: 'POST', headers: { 'Content-Type': 'text/plain' }, body: '{}',
  })).status, 415);
  assert.equal((await rawRequest(origin, '/api/v1/auth/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: 'x'.repeat(32 * 1024 + 1),
  })).status, 413);
  assert.equal(upstreamCalls, 0);
});

test('returns a JSON 502 when the configured upstream is unavailable', async (t) => {
  const closed = http.createServer();
  await new Promise((resolve) => closed.listen(0, '127.0.0.1', resolve));
  const apiTarget = `http://127.0.0.1:${closed.address().port}`;
  await new Promise((resolve) => closed.close(resolve));
  const origin = await listen(createFrontendServer({ rootDir, apiTarget }), t);
  const result = await rawRequest(origin, '/healthz');
  assert.equal(result.status, 502);
  assert.match(result.headers['content-type'], /application\/json/);
  assert.equal(JSON.parse(result.body).error.code, 'UPSTREAM_UNAVAILABLE');
});

test('returns a JSON 504 on upstream timeout and does not follow upstream redirects', async (t) => {
  let calls = 0;
  const apiTarget = await listen(http.createServer((request, response) => {
    calls += 1;
    if (request.url === '/healthz') return;
    response.writeHead(302, { Location: '/secret-path', 'Content-Type': 'application/json' });
    response.end('{"redirect":true}');
  }), t);
  const origin = await listen(createFrontendServer({ rootDir, apiTarget, timeoutMs: 80 }), t);
  const timedOut = await rawRequest(origin, '/healthz');
  assert.equal(timedOut.status, 504);
  assert.equal(JSON.parse(timedOut.body).error.code, 'UPSTREAM_TIMEOUT');
  const redirected = await rawRequest(origin, '/readyz');
  assert.equal(redirected.status, 302);
  assert.equal(redirected.headers.location, undefined);
  assert.equal(calls, 2);
});

test('requires API_TARGET to be a fixed HTTP(S) origin', () => {
  for (const apiTarget of [
    'file:///tmp/api',
    'http://user:password@localhost:8080',
    'http://localhost:8080/api',
    'http://localhost:8080?url=other',
    'http://api.example.com',
  ]) {
    assert.throws(() => createFrontendServer({ apiTarget }), /API_TARGET/);
  }
  assert.doesNotThrow(() => createFrontendServer({ apiTarget: 'https://api.example.com' }));
  assert.doesNotThrow(() => createFrontendServer({ apiTarget: 'http://127.0.0.1:8080' }));
});

test('allows only a fixed secure or loopback storage origin in CSP', () => {
  for (const storageOrigin of [
    'file:///tmp/videos',
    'http://user:password@localhost:9000',
    'http://localhost:9000/bucket',
    'http://storage.example.com',
  ]) {
    assert.throws(() => createFrontendServer({ storageOrigin }), /STORAGE_ORIGIN/);
  }
  assert.doesNotThrow(() => createFrontendServer({ storageOrigin: 'http://127.0.0.1:9000' }));
  assert.doesNotThrow(() => createFrontendServer({ storageOrigin: 'https://storage.example.com' }));
});
