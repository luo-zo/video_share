// 组合根：客户端只在应用启动时实例化一次，其余模块从这里取用，
// 避免各处各自 new 一份导致会话状态分裂。
import { createAuthClient } from './auth';
import { createCommunityClient } from './community';
import { createNotificationClient } from './notification';
import { createCreatorClient } from './creator';
import { createVideoClient } from './video';
import { createTaxonomyClient } from './taxonomy';
import { createModerationClient } from './moderation';
import { createAnalyticsClient } from './analytics';
import { createRankingClient } from './ranking';

export const authClient = createAuthClient({ persistent: true });
export const videoClient = createVideoClient({ authClient });
export const communityClient = createCommunityClient({ authClient });
export const notificationClient = createNotificationClient(authClient);
export const creatorClient = createCreatorClient({ authClient });
export const taxonomyClient = createTaxonomyClient(authClient);
export const moderationClient = createModerationClient(authClient);
export const analyticsClient = createAnalyticsClient(authClient);
export const rankingClient = createRankingClient({ authClient });
