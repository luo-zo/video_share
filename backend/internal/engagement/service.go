package engagement

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SetLike 启用或停用当前用户对某个视频的点赞，返回最终状态。缺少用户或视频时
// 在触达存储前就拒绝。
func (s *Service) SetLike(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
	return s.setRelation(ctx, userID, videoID, active, s.repo.SetLike)
}

// SetFavorite 启用或停用当前用户对某个视频的收藏，返回最终状态。
func (s *Service) SetFavorite(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
	return s.setRelation(ctx, userID, videoID, active, s.repo.SetFavorite)
}

func (s *Service) setRelation(ctx context.Context, userID, videoID uint64, active bool, write func(context.Context, uint64, uint64, bool) (*RelationState, error)) (*RelationState, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if videoID == 0 {
		return nil, ErrVideoNotFound
	}
	return write(ctx, userID, videoID, active)
}
