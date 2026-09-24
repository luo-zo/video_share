package taxonomy

type CategoryListResponse struct {
	Items []Category `json:"items"`
}

type SelectionRequest struct {
	CategoryID uint64   `json:"category_id"`
	Tags       []string `json:"tags"`
}
