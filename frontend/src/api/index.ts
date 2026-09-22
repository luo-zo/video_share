// 组合根：客户端只在应用启动时实例化一次，其余模块从这里取用，
// 避免各处各自 new 一份导致会话状态分裂。
import { createAuthClient } from './auth';
import { createCommunityClient } from './community';
import { createVideoClient } from './video';

export const authClient = createAuthClient();
export const videoClient = createVideoClient({ authClient });
export const communityClient = createCommunityClient({ authClient });
