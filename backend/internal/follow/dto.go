package follow

// FollowState 是一次关注 / 取关写入后的最终状态：当前用户是否关注了目标用户。
// 与点赞、收藏一样，重复调用返回相同结果。
type FollowState struct {
	Following bool `json:"following"`
}

// FollowedUserResponse 是关注列表中一名用户的公开字段，绝不包含密码散列或状态。
type FollowedUserResponse struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

type FollowListResponse struct {
	Items    []FollowedUserResponse `json:"items"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Total    int64                  `json:"total"`
}

func toFollowedUserResponses(users []FollowedUser) []FollowedUserResponse {
	result := make([]FollowedUserResponse, 0, len(users))
	for _, u := range users {
		result = append(result, FollowedUserResponse{ID: u.ID, Username: u.Username, Nickname: u.Nickname})
	}
	return result
}
