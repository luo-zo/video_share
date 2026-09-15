import test from 'node:test';
import assert from 'node:assert/strict';

import { createAuthClient } from '../src/auth.js';
import { CommunityError, createCommunityClient, validateComment } from '../src/community.js';

const jsonResponse = (data, status = 200) => ({
  ok: status >= 200 && status < 300,
  status,
  json: async () => data,
});

function connectedClient(apiFetch) {
  const auth = createAuthClient({ fetchImpl: apiFetch });
  return { auth, community: createCommunityClient({ authClient: auth, fetchImpl: apiFetch }) };
}

async function signIn(auth, queue) {
  queue.push(
    jsonResponse({ data: { access_token: 'session-token', token_type: 'Bearer', expires_in: 60 } }),
    jsonResponse({ data: { id: 7, username: 'milo', nickname: '小猫', created_at: '2026-09-07T12:00:00Z' } }),
  );
  await auth.signIn({ username: 'milo', password: 'password123' });
}

test('validates comment content before sending it', () => {
  assert.equal(validateComment({ content: '  很喜欢这个视频  ' }).valid, true);
  assert.equal(validateComment({ content: '  很喜欢这个视频  ' }).values.content, '很喜欢这个视频');
  assert.equal(validateComment({ content: '' }).valid, false);
  assert.equal(validateComment({ content: '   ' }).valid, false);
  assert.equal(validateComment({ content: 'x'.repeat(501) }).valid, false);
  assert.equal(validateComment({ content: 'x'.repeat(500) }).valid, true);
});

test('lists comments publicly without a session token', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    assert.ok(queue.length, `Unexpected request: ${url}`);
    return queue.shift();
  };
  const { community } = connectedClient(apiFetch);
  queue.push(jsonResponse({ data: {
    items: [{ id: 3, video_id: 9, user_id: 4, content: '第一条', created_at: '2026-09-07T12:00:00Z' }],
    page: 2,
    page_size: 12,
    total: 13,
  } }));

  const page = await community.listComments(9, { page: 2, pageSize: 12 });
  assert.equal(page.total, 13);
  assert.equal(page.items[0].content, '第一条');
  assert.deepEqual(calls.map(({ url }) => url), ['/api/v1/videos/9/comments?page=2&page_size=12']);
  assert.equal(calls[0].options.headers.Authorization, undefined);
});

test('posts and deletes comments with the session token', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    assert.ok(queue.length, `Unexpected request: ${url}`);
    return queue.shift();
  };
  const { auth, community } = connectedClient(apiFetch);
  await signIn(auth, queue);
  queue.push(
    jsonResponse({ data: { id: 11, video_id: 9, user_id: 7, content: '很棒的记录', created_at: '2026-09-07T12:00:00Z' } }, 201),
    jsonResponse({ data: { id: 11 } }),
  );

  const created = await community.addComment(9, { content: '  很棒的记录  ' });
  assert.equal(created.id, 11);
  assert.equal((await community.deleteComment(11)).id, 11);

  assert.deepEqual(calls.slice(2).map(({ url, options }) => `${options.method} ${url}`), [
    'POST /api/v1/videos/9/comments',
    'DELETE /api/v1/comments/11',
  ]);
  assert.deepEqual(JSON.parse(calls[2].options.body), { content: '很棒的记录' });
  assert.equal(calls[2].options.headers.Authorization, 'Bearer session-token');
  assert.equal(calls[3].options.headers.Authorization, 'Bearer session-token');
});

test('toggles likes and favorites with the server-returned final state', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    assert.ok(queue.length, `Unexpected request: ${url}`);
    return queue.shift();
  };
  const { auth, community } = connectedClient(apiFetch);
  await signIn(auth, queue);
  queue.push(
    jsonResponse({ data: { active: true, count: 5 } }),
    jsonResponse({ data: { active: false, count: 4 } }),
    jsonResponse({ data: { active: true, count: 2 } }),
    jsonResponse({ data: { active: false, count: 1 } }),
  );

  assert.deepEqual(await community.setLike(9, true), { active: true, count: 5 });
  assert.deepEqual(await community.setLike(9, false), { active: false, count: 4 });
  assert.deepEqual(await community.setFavorite(9, true), { active: true, count: 2 });
  assert.deepEqual(await community.setFavorite(9, false), { active: false, count: 1 });

  assert.deepEqual(calls.slice(2).map(({ url, options }) => `${options.method} ${url}`), [
    'PUT /api/v1/videos/9/like',
    'DELETE /api/v1/videos/9/like',
    'PUT /api/v1/videos/9/favorite',
    'DELETE /api/v1/videos/9/favorite',
  ]);
});

test('reports watch progress and rejects impossible progress values', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    assert.ok(queue.length, `Unexpected request: ${url}`);
    return queue.shift();
  };
  const { auth, community } = connectedClient(apiFetch);
  await signIn(auth, queue);
  queue.push(jsonResponse({ data: { video_id: 9, progress_ms: 3000, duration_ms: 10000 } }));

  const saved = await community.reportWatch(9, { progressMs: 3000, durationMs: 10000 });
  assert.equal(saved.progress_ms, 3000);
  assert.equal(calls[2].url, '/api/v1/videos/9/watch');
  assert.equal(calls[2].options.method, 'POST');
  assert.deepEqual(JSON.parse(calls[2].options.body), { progress_ms: 3000, duration_ms: 10000 });

  for (const bad of [
    { progressMs: 20000, durationMs: 10000 },
    { progressMs: -1, durationMs: 10000 },
    { progressMs: 1.5, durationMs: 10000 },
  ]) {
    await assert.rejects(community.reportWatch(9, bad), (error) => {
      assert.ok(error instanceof CommunityError);
      assert.equal(error.code, 'INVALID_PARAMETER');
      return true;
    });
  }
  assert.equal(calls.length, 3);
});

test('reports watch progress while media duration is still unknown', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    return queue.shift();
  };
  const { auth, community } = connectedClient(apiFetch);
  await signIn(auth, queue);
  queue.push(jsonResponse({ data: { video_id: 9, progress_ms: 3000, duration_ms: 0 } }));

  assert.equal((await community.reportWatch(9, { progressMs: 3000, durationMs: 0, keepalive: true })).duration_ms, 0);
  assert.deepEqual(JSON.parse(calls[2].options.body), { progress_ms: 3000, duration_ms: 0 });
  assert.equal(calls[2].options.keepalive, true);
});

test('lists favorites, history and follows with pagination', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    assert.ok(queue.length, `Unexpected request: ${url}`);
    return queue.shift();
  };
  const { auth, community } = connectedClient(apiFetch);
  await signIn(auth, queue);
  queue.push(
    jsonResponse({ data: { items: [], page: 1, page_size: 12, total: 0 } }),
    jsonResponse({ data: { items: [], page: 1, page_size: 12, total: 0 } }),
    jsonResponse({ data: { items: [{ id: 4, username: 'luna', nickname: '月亮' }], page: 1, page_size: 12, total: 1 } }),
  );

  assert.deepEqual((await community.listFavorites()).items, []);
  assert.deepEqual((await community.listHistory()).items, []);
  assert.equal((await community.listFollows()).items[0].username, 'luna');

  assert.deepEqual(calls.slice(2).map(({ url }) => url), [
    '/api/v1/users/me/favorites?page=1&page_size=12',
    '/api/v1/users/me/history?page=1&page_size=12',
    '/api/v1/users/me/follows?page=1&page_size=12',
  ]);
});

test('toggles follows with the server-returned final state', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    assert.ok(queue.length, `Unexpected request: ${url}`);
    return queue.shift();
  };
  const { auth, community } = connectedClient(apiFetch);
  await signIn(auth, queue);
  queue.push(
    jsonResponse({ data: { following: true } }),
    jsonResponse({ data: { following: false } }),
  );

  assert.equal((await community.setFollow(4, true)).following, true);
  assert.equal((await community.setFollow(4, false)).following, false);
  assert.deepEqual(calls.slice(2).map(({ url, options }) => `${options.method} ${url}`), [
    'PUT /api/v1/users/4/follow',
    'DELETE /api/v1/users/4/follow',
  ]);
});

test('rejects malformed identifiers and responses from the community API', async () => {
  const queue = [];
  const apiFetch = async () => queue.shift();
  const { auth, community } = connectedClient(apiFetch);
  await signIn(auth, queue);

  await assert.rejects(community.setLike(0, true), (error) => {
    assert.ok(error instanceof CommunityError);
    assert.equal(error.code, 'INVALID_PARAMETER');
    return true;
  });
  await assert.rejects(community.listComments('nope'), { code: 'INVALID_PARAMETER' });

  queue.push(jsonResponse({ data: { active: 'yes' } }));
  await assert.rejects(community.setLike(9, true), { code: 'INVALID_RESPONSE' });
});
