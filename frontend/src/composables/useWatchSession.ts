import type { Ref } from 'vue';

import { analyticsClient } from '../api';

interface WatchSessionOptions {
  readonly element: Ref<HTMLVideoElement | null>;
  readonly enabled: () => boolean;
}

// Playback sampling is wall-clock based. Progress deltas and playbackRate are
// deliberately ignored, so seeking, buffering and 2x playback cannot mint
// extra effective watch time.
export function useWatchSession({ element, enabled }: WatchSessionOptions) {
  let sessionID = '';
  let generation = 0;
  let sequence = 0;
  let resumePositionMS = 0;
  let resumeApplied = false;
  let activeSince = 0;
  let timer: ReturnType<typeof setInterval> | null = null;
  let currentVideoID: string | number | null = null;
  let suspended = false;
  let pending = Promise.resolve();

  const visible = (): boolean => typeof document === 'undefined' || document.visibilityState === 'visible';

  function resetActivity(): void {
    activeSince = 0;
  }

  function beginActivity(): void {
    const video = element.value;
    if (video && !video.paused && !video.ended && visible() && activeSince === 0) activeSince = Date.now();
  }

  function queueHeartbeat(ended = false, boundary = false): void {
    const video = element.value;
    const localSession = sessionID;
    if (!video || !localSession || !currentVideoID) return;
    const hidden = !visible() && !ended && !boundary;
    const inactive = !ended && !boundary && (suspended || video.paused || video.ended);
    if (hidden || inactive) return;
    const now = Date.now();
    const delta = activeSince > 0 ? Math.min(Math.max(0, now - activeSince), 15_000) : 0;
    activeSince = ended ? 0 : now;
    const seq = ++sequence;
    const input = {
      seq,
      position_ms: Math.max(0, Math.floor((Number.isFinite(video.currentTime) ? video.currentTime : 0) * 1000)),
      watched_delta_ms: Math.floor(delta),
      ended,
    };
    pending = pending.then(() => analyticsClient.heartbeat(localSession, input, { keepalive: ended }).then(() => undefined)).catch(() => undefined);
  }

  function applyResume(): void {
    const video = element.value;
    if (resumeApplied || !video || resumePositionMS <= 0 || !Number.isFinite(video.duration) || video.duration <= 0) return;
    resumeApplied = true;
    const durationMS = video.duration * 1000;
    if (resumePositionMS + 2000 < durationMS && resumePositionMS < durationMS) {
      video.currentTime = Math.max(0, Math.min(resumePositionMS / 1000, Math.max(0, video.duration - 0.25)));
    }
  }

  const onPlay = (): void => { suspended = false; beginActivity(); };
  const onPlaying = (): void => { suspended = false; beginActivity(); };
  const onPause = (): void => { queueHeartbeat(false, true); suspended = true; resetActivity(); };
  const onWaiting = (): void => { queueHeartbeat(false, true); suspended = true; resetActivity(); };
  const onSeeking = (): void => { queueHeartbeat(false, true); suspended = true; resetActivity(); };
  const onSeeked = (): void => { suspended = false; beginActivity(); };
  const onEnded = (): void => { queueHeartbeat(true); suspended = true; resetActivity(); };

  function bind(): void {
    const video = element.value;
    if (!video) return;
    video.addEventListener('loadedmetadata', applyResume);
    video.addEventListener('play', onPlay);
    video.addEventListener('playing', onPlaying);
    video.addEventListener('pause', onPause);
    video.addEventListener('waiting', onWaiting);
    video.addEventListener('seeking', onSeeking);
    video.addEventListener('seeked', onSeeked);
    video.addEventListener('ended', onEnded);
    document.addEventListener('visibilitychange', onVisibilityChange);
    timer = setInterval(() => queueHeartbeat(), 5000);
  }

  function unbind(): void {
    const video = element.value;
    if (video) {
      video.removeEventListener('loadedmetadata', applyResume);
      video.removeEventListener('play', onPlay);
      video.removeEventListener('playing', onPlaying);
      video.removeEventListener('pause', onPause);
      video.removeEventListener('waiting', onWaiting);
      video.removeEventListener('seeking', onSeeking);
      video.removeEventListener('seeked', onSeeked);
      video.removeEventListener('ended', onEnded);
    }
    document.removeEventListener('visibilitychange', onVisibilityChange);
    if (timer !== null) clearInterval(timer);
    timer = null;
  }

  function onVisibilityChange(): void {
    if (visible()) { suspended = false; beginActivity(); }
    else {
      queueHeartbeat(false, true);
      suspended = true;
      resetActivity();
    }
  }

  async function start(videoID: string | number): Promise<void> {
    stop();
    if (!enabled()) return;
    currentVideoID = videoID;
    const current = ++generation;
    try {
      const started = await analyticsClient.startWatchSession(videoID);
      if (current !== generation || !enabled() || currentVideoID !== videoID) return;
      sessionID = started.session_id;
      sequence = 0;
      resumePositionMS = Number(started.resume_position_ms) || 0;
      resumeApplied = false;
      suspended = false;
      bind();
      applyResume();
    } catch {
      if (current === generation) {
        sessionID = '';
        currentVideoID = null;
      }
    }
  }

  function stop(): void {
    if (sessionID) queueHeartbeat(false, true);
    generation += 1;
    unbind();
    sessionID = '';
    currentVideoID = null;
    sequence = 0;
    resumePositionMS = 0;
    resumeApplied = false;
    suspended = false;
    resetActivity();
  }

  return { start, stop, applyResume };
}
