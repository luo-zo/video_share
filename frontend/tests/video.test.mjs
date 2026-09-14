import test from 'node:test';
import assert from 'node:assert/strict';

import { AuthError, createAuthClient } from '../src/auth.js';
import { VideoError, createVideoClient, validateVideoSubmission } from '../src/video.js';

const jsonResponse = (data, status = 200) => ({
  ok: status >= 200 && status < 300,
  status,
  json: async () => data,
});

function authenticatedClient(apiFetch, uploadFetch = apiFetch) {
  const auth = createAuthClient({ fetchImpl: apiFetch });
  return {
    auth,
    video: createVideoClient({ authClient: auth, fetchImpl: uploadFetch }),
  };
}

async function signIn(auth, apiQueue) {
  apiQueue.push(
    jsonResponse({ data: { access_token: 'session-token', token_type: 'Bearer', expires_in: 60 } }),
    jsonResponse({ data: { id: 7, username: 'milo', nickname: '小猫', created_at: '2026-09-07T12:00:00Z' } }),
  );
  await auth.signIn({ username: 'milo', password: 'password123' });
}

test('validates a title and an MP4 file before upload', () => {
  assert.equal(validateVideoSubmission({ title: '  猫咪散步  ', description: '  傍晚记录  ', file: {
    name: 'walk.MP4', type: '', size: 4096,
  } }).valid, true);

  for (const input of [
    { title: '', file: { name: 'walk.mp4', type: 'video/mp4', size: 1 } },
    { title: '猫咪', file: null },
    { title: '猫咪', file: { name: 'walk.mov', type: 'video/quicktime', size: 1 } },
    { title: '猫咪', file: { name: 'walk.mp4', type: 'video/mp4', size: 0 } },
  ]) {
    assert.equal(validateVideoSubmission(input).valid, false);
  }
});

test('lists discovery, public detail and current-user video detail with bearer session requests', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    assert.ok(queue.length, `Unexpected request: ${url}`);
    return queue.shift();
  };
  const { auth, video } = authenticatedClient(apiFetch);
  await signIn(auth, queue);
  queue.push(
    jsonResponse({ data: { items: [{ id: 1, title: '第一帧', status: 'ready', author: { id: 7, nickname: '小猫' } }], page: 2, page_size: 6, total: 7 } }),
    jsonResponse({ data: { id: 1, title: '第一帧', status: 'ready', play_url: '/media/1.mp4', play_type: 'mp4' } }),
    jsonResponse({ data: { items: [], page: 1, page_size: 12, total: 0 } }),
    jsonResponse({ data: { id: 2, title: '转码中', status: 'processing', processing_progress: 75 } }),
  );

  assert.equal((await video.listVideos({ page: 2, pageSize: 6 })).items[0].title, '第一帧');
  assert.equal((await video.getVideo(1)).play_url, '/media/1.mp4');
  assert.deepEqual((await video.listMyVideos()).items, []);
  assert.equal((await video.getMyVideo(2)).processing_progress, 75);

  assert.deepEqual(calls.slice(2).map(({ url }) => url), [
    '/api/v1/videos?page=2&page_size=6',
    '/api/v1/videos/1',
    '/api/v1/users/me/videos?page=1&page_size=12',
    '/api/v1/users/me/videos/2',
  ]);
  for (const { options } of calls.slice(2)) {
    assert.equal(options.headers.Authorization, 'Bearer session-token');
  }
});

test('uploads in create, direct PUT and complete order without sending bearer token to upload URL', async () => {
  const apiQueue = [];
  const events = [];
  const apiCalls = [];
  const apiFetch = async (url, options) => {
    apiCalls.push({ url, options });
    assert.ok(apiQueue.length, `Unexpected API request: ${url}`);
    return apiQueue.shift();
  };
  const uploadCalls = [];
  const uploadFetch = async (url, options) => {
    uploadCalls.push({ url, options });
    return { ok: true, status: 200 };
  };
  const { auth, video } = authenticatedClient(apiFetch, uploadFetch);
  await signIn(auth, apiQueue);
  apiQueue.push(
    jsonResponse({ data: { id: 23, status: 'uploading', upload_url: 'https://uploads.example/23', upload_expires_in: 300 } }, 201),
    jsonResponse({ data: { id: 23, status: 'processing', processing_progress: 0 } }, 202),
  );
  const file = { name: 'cat.mp4', type: 'video/mp4', size: 8192 };

  const result = await video.uploadVideo({ title: '  猫咪的一天 ', description: ' 午后 ', file }, {
    onStep: (step) => events.push(step),
  });

  assert.equal(result.id, 23);
  assert.equal(result.status, 'processing');
  assert.deepEqual(events, ['creating', 'uploading', 'completing', 'processing']);
  assert.deepEqual(JSON.parse(apiCalls[2].options.body), {
    title: '猫咪的一天', description: '午后', file_name: 'cat.mp4', content_type: 'video/mp4', file_size: 8192,
  });
  assert.equal(uploadCalls[0].url, 'https://uploads.example/23');
  assert.equal(uploadCalls[0].options.method, 'PUT');
  assert.equal(uploadCalls[0].options.body, file);
  assert.equal(uploadCalls[0].options.headers.Authorization, undefined);
  assert.equal(apiCalls[3].url, '/api/v1/videos/23/complete');
});

test('polls owner detail until processing reaches a terminal state', async () => {
  const queue = [];
  const calls = [];
  const apiFetch = async (url, options) => {
    calls.push({ url, options });
    return queue.shift();
  };
  const updates = [];
  const { auth } = authenticatedClient(apiFetch);
  await signIn(auth, queue);
  const video = createVideoClient({
    authClient: auth,
    fetchImpl: apiFetch,
    sleep: async () => {},
    now: (() => { let value = 0; return () => value += 100; })(),
  });
  queue.push(
    jsonResponse({ data: { id: 23, status: 'processing', processing_progress: 20 } }),
    jsonResponse({ data: { id: 23, status: 'processing', processing_progress: 75 } }),
    jsonResponse({ data: { id: 23, status: 'ready', processing_progress: 100 } }),
  );

  const result = await video.waitUntilProcessed(23, {
    intervalMs: 1,
    timeoutMs: 1000,
    onUpdate: (item) => updates.push(item.processing_progress),
  });

  assert.equal(result.status, 'ready');
  assert.deepEqual(updates, [20, 75, 100]);
  assert.deepEqual(calls.slice(2).map(({ url }) => url), [
    '/api/v1/users/me/videos/23',
    '/api/v1/users/me/videos/23',
    '/api/v1/users/me/videos/23',
  ]);
});

test('stops polling when the worker reports failure', async () => {
  const queue = [];
  const apiFetch = async () => queue.shift();
  const { auth } = authenticatedClient(apiFetch);
  await signIn(auth, queue);
  const video = createVideoClient({ authClient: auth, fetchImpl: apiFetch, sleep: async () => {} });
  queue.push(jsonResponse({ data: {
    id: 9, status: 'failed', processing_progress: 0, processing_error: 'ffmpeg failed',
  } }));

  const result = await video.waitUntilProcessed(9);
  assert.equal(result.status, 'failed');
  assert.equal(result.processing_error, 'ffmpeg failed');
});

test('an API 401 clears the in-memory auth session', async () => {
  const queue = [];
  const apiFetch = async () => queue.shift();
  const { auth, video } = authenticatedClient(apiFetch);
  await signIn(auth, queue);
  queue.push(jsonResponse({ error: { code: 'INVALID_TOKEN' } }, 401));

  await assert.rejects(video.listVideos(), (error) => {
    assert.ok(error instanceof AuthError);
    assert.equal(error.status, 401);
    return true;
  });
  assert.equal(auth.getSession(), null);
});

test('invalid upload input and failed direct uploads stop before completion', async () => {
  const queue = [];
  const apiCalls = [];
  const apiFetch = async (url, options) => {
    apiCalls.push({ url, options });
    return queue.shift();
  };
  const { auth, video } = authenticatedClient(apiFetch, async () => ({ ok: false, status: 403 }));
  await signIn(auth, queue);

  await assert.rejects(video.uploadVideo({ title: '', file: null }), (error) => {
    assert.ok(error instanceof VideoError);
    assert.equal(error.code, 'INVALID_PARAMETER');
    return true;
  });
  assert.equal(apiCalls.length, 2);

  queue.push(jsonResponse({ data: { id: 9, status: 'uploading', upload_url: 'https://uploads.example/9', upload_expires_in: 60 } }, 201));
  await assert.rejects(video.uploadVideo({
    title: '失败示例', file: { name: 'fail.mp4', type: 'video/mp4', size: 4 },
  }), { code: 'UPLOAD_FAILED', status: 403 });
  assert.equal(apiCalls.length, 3);
});
