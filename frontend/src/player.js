import { VideoError } from './video.js';

export async function attachVideoSource(video, item, {
  loadHls = () => import('/vendor/hls.mjs'),
  onError = () => {},
} = {}) {
  if (!video || typeof video.canPlayType !== 'function' || typeof item?.play_url !== 'string' || !item.play_url) {
    throw new VideoError('服务器没有返回有效的播放地址。', { code: 'INVALID_RESPONSE' });
  }

  const attachDirectly = () => {
    video.src = item.play_url;
    return () => { video.removeAttribute?.('src'); video.load?.(); };
  };

  if (item.play_type !== 'hls') {
    return attachDirectly();
  }

  const nativeHLS = Boolean(
    video.canPlayType('application/vnd.apple.mpegurl')
      || video.canPlayType('application/x-mpegURL'),
  );
  let module;
  try {
    module = await loadHls();
  } catch {
    if (nativeHLS) return attachDirectly();
    throw new VideoError('HLS 播放组件加载失败，请刷新页面后重试。', {
      code: 'HLS_PLAYER_LOAD_FAILED',
    });
  }

  const Hls = module?.default;
  if (Hls && typeof Hls.isSupported === 'function' && Hls.isSupported()) {
    const hls = new Hls({ enableWorker: false });
    if (Hls.Events?.ERROR && typeof hls.on === 'function') {
      hls.on(Hls.Events.ERROR, (_event, data) => onError({
        fatal: Boolean(data?.fatal),
        type: data?.type || '',
        details: data?.details || '',
        status: Number(data?.response?.code) || 0,
        reason: data?.reason || data?.error?.message || '',
      }));
    }
    hls.loadSource(item.play_url);
    hls.attachMedia(video);
    return () => hls.destroy();
  }
  if (nativeHLS) return attachDirectly();
  throw new VideoError('当前浏览器不支持 HLS 视频播放。', { code: 'HLS_UNSUPPORTED' });
}
