package follow

import "context"

const (
	defaultPageSize = 12
	maxPageSize     = 50
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Follow 让当前用户关注目标用户，返回最终关注状态。自关注在触达存储前就拒绝。
func (s *Service) Follow(ctx context.Context, followerID, followeeID uint64) (*FollowState, error) {
	return s.setFollow(ctx, followerID, followeeID, true)
}

// Unfollow 取消当前用户对目标用户的关注，返回最终关注状态。
func (s *Service) Unfollow(ctx context.Context, followerID, followeeID uint64) (*FollowState, error) {
	return s.setFollow(ctx, followerID, followeeID, false)
}

func (s *Service) setFollow(ctx context.Context, followerID, followeeID uint64, active bool) (*FollowState, error) {
	if followerID == 0 {
		return nil, ErrUnauthorized
	}
	if followeeID == 0 {
		return nil, ErrUserNotFound
	}
	if followerID == followeeID {
		return nil, ErrSelfFollow
	}
	return s.repo.SetFollow(ctx, followerID, followeeID, active)
}

func (s *Service) ListFollows(ctx context.Context, followerID uint64, page, pageSize int) (*FollowListResponse, error) {
	if followerID == 0 {
		return nil, ErrUnauthorized
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	users, total, err := s.repo.ListFollows(ctx, followerID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &FollowListResponse{
		Items:    toFollowedUserResponses(users),
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func validatePagination(page, pageSize int) error {
	if page < 1 || pageSize < 1 || pageSize > maxPageSize {
		return ErrPaginationInvalid
	}
	return nil
}
