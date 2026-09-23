import { VideoError } from '../api/video';
import type { VideoItem } from '../api/video';

export interface HlsErrorInfo {
  readonly fatal: boolean;
  readonly type: string;
  readonly details: string;
  readonly status: number;
  readonly reason: string;
}

interface HlsErrorData {
  readonly fatal?: unknown;
  readonly type?: unknown;
  readonly details?: unknown;
  readonly reason?: unknown;
  readonly response?: { readonly code?: unknown };
  readonly error?: { readonly message?: unknown };
}

interface HlsInstance {
  on(event: string, handler: (event: string, data: HlsErrorData) => void): void;
  loadSource(url: string): void;
  attachMedia(media: HTMLVideoElement): void;
  destroy(): void;
}

interface HlsModule {
  default: {
    isSupported(): boolean;
    Events?: { ERROR?: string };
    new (config?: { enableWorker?: boolean }): HlsInstance;
  };
}

export type Detach = () => void;

export interface AttachOptions {
  loadHls?: () => Promise<HlsModule>;
  onError?: (info: HlsErrorInfo) => void;
}

// 走 hls.js 的 light 构建：完整构建会打进字幕、备用音轨、EME 与 CMCD 支持，
// 而本播放器只用基础 MSE 拉流，用不到的部分让该 chunk 长期超过 Vite 的 500 kB 警戒线。
// 代价是无法再播放这些特性，如果将来要支持字幕需要切回 'hls.js'。
// hls.js 的实际模块形状比这里最小用到的接口宽，只在这一处收口转换。
const loadBundledHls = async (): Promise<HlsModule> => (await import('hls.js/light')) as unknown as HlsModule;

function readString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

/**
 * 把视频源接到 <video> 上，返回清理函数。清理函数必须负责销毁 HLS 实例，
 * 否则切换视频时会留下继续拉流的监听器。
 */
export async function attachVideoSource(
  video: HTMLVideoElement | null,
  item: VideoItem | null,
  { loadHls = loadBundledHls, onError = () => {} }: AttachOptions = {},
): Promise<Detach> {
  if (!video || typeof video.canPlayType !== 'function') {
    throw new VideoError('服务器没有返回有效的播放地址。', { code: 'INVALID_RESPONSE' });
  }
  const playUrl = typeof item?.play_url === 'string' ? item.play_url : '';
  if (!playUrl) {
    throw new VideoError('服务器没有返回有效的播放地址。', { code: 'INVALID_RESPONSE' });
  }

  const attachDirectly = (): Detach => {
    video.src = playUrl;
    return () => {
      video.removeAttribute?.('src');
      video.load?.();
    };
  };

  if (item?.play_type !== 'hls') return attachDirectly();

  const nativeHLS = Boolean(
    video.canPlayType('application/vnd.apple.mpegurl')
      || video.canPlayType('application/x-mpegURL'),
  );

  let module: HlsModule;
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
    try {
      if (Hls.Events?.ERROR && typeof hls.on === 'function') {
        hls.on(Hls.Events.ERROR, (_event, data) => onError({
          fatal: Boolean(data?.fatal),
          type: readString(data?.type),
          details: readString(data?.details),
          status: Number(data?.response?.code) || 0,
          reason: readString(data?.reason) || readString(data?.error?.message),
        }));
      }
      hls.loadSource(playUrl);
      hls.attachMedia(video);
    } catch {
      hls.destroy?.();
      throw new VideoError('HLS 播放器初始化失败，请刷新页面后重试。', {
        code: 'HLS_PLAYER_INIT_FAILED',
      });
    }
    return () => hls.destroy();
  }
  if (nativeHLS) return attachDirectly();
  throw new VideoError('当前浏览器不支持 HLS 视频播放。', { code: 'HLS_UNSUPPORTED' });
}
