import test from 'node:test';
import assert from 'node:assert/strict';
import { AuthError, createAuthClient, validateLogin, validateRegistration } from '../src/auth.js';

const user = { id: 7, username: 'milo', nickname: '小猫', created_at: '2026-09-07T12:00:00Z' };
const credentials = { username: ' MILO ', password: '  secret pass  ' };
const registration = { ...credentials, nickname: ' 小猫 ' };
const tokenData = { access_token: 'test-token', token_type: 'Bearer', expires_in: 60 };
const response = (data, status = 200) => ({ ok: status >= 200 && status < 300, status, json: async () => data });

function clientWithQueue(queue, options = {}) {
  const calls = [];
  const client = createAuthClient({
    ...options,
    fetchImpl: async (url, request) => {
      calls.push({ url, request });
      assert.ok(queue.length, `Unexpected request: ${url}`);
      const next = queue.shift();
      if (typeof next === 'function') return next(url, request);
      return next;
    },
  });
  return { client, calls };
}

test('login normalizes usernames but preserves passwords and permits short legacy passwords', () => {
  assert.deepEqual(validateLogin({ username: ' Ab_C ', password: ' x ' }), {
    values: { username: 'ab_c', password: ' x ' }, errors: {}, valid: true,
  });
  assert.equal(validateLogin({ username: 'a', password: '1' }).valid, true);
  assert.deepEqual(Object.keys(validateLogin({ username: ' ', password: '' }).errors), ['username', 'password']);
});

test('registration enforces ASCII username boundaries and trims nickname without trimming password', () => {
  const valid = validateRegistration(registration);
  assert.equal(valid.valid, true);
  assert.deepEqual(valid.values, { username: 'milo', password: '  secret pass  ', nickname: '小猫' });
  for (const username of ['ab', 'a'.repeat(33), '中文账号', 'a-b', 'a b']) {
    assert.ok(validateRegistration({ ...registration, username }).errors.username);
  }
  for (const username of ['a_b', 'a'.repeat(32)]) {
    assert.equal(validateRegistration({ ...registration, username }).errors.username, undefined);
  }
});

test('registration counts Unicode codepoints and UTF-8 bytes independently', () => {
  assert.ok(validateRegistration({ ...registration, password: '🐱'.repeat(7) }).errors.password);
  assert.equal(validateRegistration({ ...registration, password: '🐱'.repeat(8) }).valid, true);
  assert.equal(validateRegistration({ ...registration, password: '猫'.repeat(24) }).valid, true);
  assert.ok(validateRegistration({ ...registration, password: '猫'.repeat(25) }).errors.password);
  assert.equal(validateRegistration({ ...registration, password: 'a'.repeat(72) }).valid, true);
  assert.ok(validateRegistration({ ...registration, password: 'a'.repeat(73) }).errors.password);
  assert.equal(validateRegistration({ ...registration, nickname: '🐱'.repeat(64) }).valid, true);
  assert.ok(validateRegistration({ ...registration, nickname: '🐱'.repeat(65) }).errors.nickname);
  assert.ok(validateRegistration({ ...registration, nickname: '   ' }).errors.nickname);
});

test('invalid input is rejected before making a network request', async () => {
  const { client, calls } = clientWithQueue([]);
  await assert.rejects(client.register({ username: 'x', password: '1', nickname: '' }), (error) => {
    assert.ok(error instanceof AuthError);
    assert.equal(error.code, 'INVALID_PARAMETER');
    assert.ok(error.fieldErrors.username);
    return true;
  });
  await assert.rejects(client.signIn({ username: '', password: '' }), { code: 'INVALID_PARAMETER' });
  assert.equal(calls.length, 0);
});

test('registration sends normalized values, returns the user, and does not create a session', async () => {
  const { client, calls } = clientWithQueue([response({ data: user }, 201)]);
  assert.deepEqual(await client.register(registration), user);
  assert.equal(client.getSession(), null);
  assert.equal(calls[0].url, '/api/v1/auth/register');
  assert.equal(calls[0].request.method, 'POST');
  assert.equal(calls[0].request.credentials, 'omit');
  assert.deepEqual(JSON.parse(calls[0].request.body), {
    username: 'milo', password: credentials.password, nickname: '小猫',
  });
  assert.equal(calls[0].request.headers.Authorization, undefined);
});

test('login requires a bearer-authenticated profile before committing any session', async () => {
  let resolveProfile;
  const { client, calls } = clientWithQueue([
    response({ data: tokenData }),
    () => new Promise((resolve) => { resolveProfile = resolve; }),
  ], { now: () => 1000 });
  const pending = client.signIn(credentials);
  while (!resolveProfile) await new Promise((resolve) => setImmediate(resolve));
  assert.equal(client.getSession(), null);
  assert.equal(calls[1].url, '/api/v1/users/me');
  assert.equal(calls[1].request.headers.Authorization, 'Bearer test-token');
  assert.equal(calls[1].request.body, undefined);
  resolveProfile(response({ data: user }));
  assert.deepEqual(await pending, user);
  assert.deepEqual(client.getSession(), { user, expiresAt: 61000 });
  assert.equal('token' in client.getSession(), false);
  assert.equal('password' in client.getSession(), false);
});

test('failed profile retrieval never leaves an authenticated session', async () => {
  const { client } = clientWithQueue([
    response({ data: tokenData }),
    response({ error: { code: 'UNAUTHORIZED', message: 'not authorized' } }, 401),
  ]);
  await assert.rejects(client.signIn(credentials), { code: 'UNAUTHORIZED', status: 401 });
  assert.equal(client.getSession(), null);
});

test('expired sessions are removed before a profile request and cannot be reused', async () => {
  let clock = 1000;
  const { client, calls } = clientWithQueue([
    response({ data: tokenData }), response({ data: user }),
  ], { now: () => clock });
  await client.signIn(credentials);
  clock = 60999;
  assert.ok(client.getSession());
  clock = 61000;
  assert.equal(client.getSession(), null);
  await assert.rejects(client.getProfile(), { code: 'SESSION_EXPIRED' });
  assert.equal(calls.length, 2);
});

test('sign out cancels a pending sign in so its late response cannot restore a session', async () => {
  let resolveProfile;
  const { client } = clientWithQueue([
    response({ data: tokenData }),
    () => new Promise((resolve) => { resolveProfile = resolve; }),
  ]);
  const pending = client.signIn(credentials);
  while (!resolveProfile) await new Promise((resolve) => setImmediate(resolve));
  client.signOut();
  resolveProfile(response({ data: user }));
  await assert.rejects(pending, { code: 'REQUEST_CANCELLED' });
  assert.equal(client.getSession(), null);
});

test('sign out clears a completed session and 401 clears an active profile session', async () => {
  const { client } = clientWithQueue([
    response({ data: tokenData }), response({ data: user }),
    response({ error: { code: 'INVALID_TOKEN' } }, 401),
    response({ data: tokenData }), response({ data: user }),
  ]);
  await client.signIn(credentials);
  await assert.rejects(client.getProfile(), { status: 401 });
  assert.equal(client.getSession(), null);
  await client.signIn(credentials);
  client.signOut();
  assert.equal(client.getSession(), null);
});

test('API errors retain diagnostic identifiers and provide specific Chinese messages', async (t) => {
  const cases = [
    { status: 401, code: 'INVALID_CREDENTIALS', message: /用户名或密码不正确/ },
    { status: 409, code: 'USER_ALREADY_EXISTS', message: /用户名已被使用/ },
    { status: 429, code: 'RATE_LIMIT_EXCEEDED', message: /操作太频繁/ },
    { status: 400, code: 'INVALID_PARAMETER', message: /输入信息不符合要求/ },
    { status: 409, code: 'UPLOAD_INCOMPLETE', message: /尚未上传完成/ },
    { status: 409, code: 'UPLOAD_MISMATCH', message: /投稿信息不一致/ },
    { status: 409, code: 'VIDEO_STATE_CONFLICT', message: /当前视频状态/ },
    { status: 503, code: 'UNAVAILABLE', message: /服务暂时不可用/ },
  ];
  for (const item of cases) {
    await t.test(item.code, async () => {
      const { client } = clientWithQueue([response({
        error: { code: item.code, message: 'server detail', request_id: 'request-123' },
      }, item.status)]);
      await assert.rejects(client.signIn(credentials), (error) => {
        assert.equal(error.code, item.code);
        assert.equal(error.status, item.status);
        assert.equal(error.requestId, 'request-123');
        assert.match(error.message, item.message);
        return true;
      });
    });
  }
});

test('network failures and request timeouts are distinct and do not create a session', async () => {
  const offline = createAuthClient({ fetchImpl: async () => { throw new TypeError('Failed to fetch'); } });
  await assert.rejects(offline.signIn(credentials), { code: 'NETWORK_ERROR' });
  assert.equal(offline.getSession(), null);
  const slow = createAuthClient({
    timeoutMs: 5,
    fetchImpl: (_url, { signal }) => new Promise((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true });
    }),
  });
  await assert.rejects(slow.signIn(credentials), { code: 'REQUEST_TIMEOUT' });
  assert.equal(slow.getSession(), null);
});

test('malformed JSON and invalid token/profile responses fail honestly', async () => {
  for (const data of [
    {},
    { data: { ...tokenData, access_token: '' } },
    { data: { ...tokenData, token_type: 'Basic' } },
    { data: { ...tokenData, expires_in: 0 } },
    { data: { ...tokenData, expires_in: '60' } },
  ]) {
    const { client } = clientWithQueue([response(data)]);
    await assert.rejects(client.signIn(credentials), { code: 'INVALID_RESPONSE' });
    assert.equal(client.getSession(), null);
  }
  const { client: badJson } = clientWithQueue([{
    ok: true, status: 200, json: async () => { throw new SyntaxError('Unexpected HTML'); },
  }]);
  await assert.rejects(badJson.signIn(credentials), { code: 'INVALID_RESPONSE' });
  const { client: badProfile } = clientWithQueue([response({ data: tokenData }), response({ data: {} })]);
  await assert.rejects(badProfile.signIn(credentials), { code: 'INVALID_RESPONSE' });
  assert.equal(badProfile.getSession(), null);
});
