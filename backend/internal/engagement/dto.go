package engagement

// RelationState 是一次点赞 / 收藏写入后的最终状态：当前用户是否处于该关系中，
// 以及该关系的最新总数。重复调用返回同样的结果。
type RelationState struct {
	Active bool   `json:"active"`
	Count  uint64 `json:"count"`
}
