import assert from 'node:assert/strict';
import test from 'node:test';

import { attachVideoSource } from '../src/player.js';

function fakeVideo(nativeHLS = false) {
  return {
    src: '',
    canPlayType: (type) => nativeHLS && type === 'application/vnd.apple.mpegurl' ? 'maybe' : '',
  };
}

test('uses the MP4 URL directly for migration-era videos', async () => {
  const video = fakeVideo();
  const cleanup = await attachVideoSource(video, { play_type: 'mp4', play_url: '/media/video.mp4' }, {
    loadHls: async () => { throw new Error('Hls should not load for MP4'); },
  });
  assert.equal(video.src, '/media/video.mp4');
  cleanup();
});

test('uses native HLS playback when the browser supports it', async () => {
  class UnsupportedHls { static isSupported() { return false; } }
  const video = fakeVideo(true);
  const cleanup = await attachVideoSource(video, { play_type: 'hls', play_url: '/api/v1/videos/7/hls/master.m3u8' }, {
    loadHls: async () => ({ default: UnsupportedHls }),
  });
  assert.equal(video.src, '/api/v1/videos/7/hls/master.m3u8');
  cleanup();
});

test('uses hls.js for browsers with Media Source support and returns cleanup', async () => {
  const calls = [];
  class FakeHls {
    static isSupported() { return true; }
    constructor(options) { calls.push(['construct', options]); }
    loadSource(url) { calls.push(['loadSource', url]); }
    attachMedia(video) { calls.push(['attachMedia', video]); }
    destroy() { calls.push(['destroy']); }
  }
  const video = fakeVideo(false);
  const cleanup = await attachVideoSource(video, { play_type: 'hls', play_url: '/api/v1/videos/7/hls/master.m3u8' }, {
    loadHls: async () => ({ default: FakeHls }),
  });
  assert.deepEqual(calls.slice(0, 3), [
    ['construct', { enableWorker: false }],
    ['loadSource', '/api/v1/videos/7/hls/master.m3u8'],
    ['attachMedia', video],
  ]);
  cleanup();
  assert.deepEqual(calls.at(-1), ['destroy']);
});

test('prefers hls.js when a Chromium-like browser falsely claims native HLS support', async () => {
  const calls = [];
  let errorHandler;
  const errors = [];
  class FakeHls {
    static Events = { ERROR: 'hls-error' };
    static isSupported() { return true; }
    on(event, handler) { calls.push(['on', event]); errorHandler = handler; }
    loadSource(url) { calls.push(['loadSource', url]); }
    attachMedia(video) { calls.push(['attachMedia', video]); }
    destroy() { calls.push(['destroy']); }
  }
  const video = fakeVideo(true);
  const cleanup = await attachVideoSource(video, {
    play_type: 'hls',
    play_url: '/api/v1/videos/7/hls/master.m3u8',
  }, {
    loadHls: async () => ({ default: FakeHls }),
    onError: (error) => errors.push(error),
  });

  assert.deepEqual(calls.slice(0, 3), [
    ['on', 'hls-error'],
    ['loadSource', '/api/v1/videos/7/hls/master.m3u8'],
    ['attachMedia', video],
  ]);
  assert.equal(video.src, '');
  errorHandler('hls-error', {
    fatal: true,
    type: 'mediaError',
    details: 'bufferAppendError',
    response: { code: 500 },
    error: new Error('append failed'),
  });
  assert.deepEqual(errors, [{
    fatal: true,
    type: 'mediaError',
    details: 'bufferAppendError',
    status: 500,
    reason: 'append failed',
  }]);
  cleanup();
  assert.deepEqual(calls.at(-1), ['destroy']);
});

test('reports a useful error when HLS is unsupported', async () => {
  class UnsupportedHls { static isSupported() { return false; } }
  await assert.rejects(
    attachVideoSource(fakeVideo(false), { play_type: 'hls', play_url: '/master.m3u8' }, {
      loadHls: async () => ({ default: UnsupportedHls }),
    }),
    /不支持 HLS/,
  );
});
