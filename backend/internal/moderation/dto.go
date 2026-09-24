package moderation

import "time"

type CreateReportRequest struct {
	TargetType string `json:"target_type"`
	TargetID   uint64 `json:"target_id"`
	ReasonCode string `json:"reason_code"`
	Detail     string `json:"detail"`
	RequestID  string `json:"request_id"`
}

type ReportDecisionRequest struct {
	Status           string `json:"status"`
	ResolutionReason string `json:"resolution_reason"`
	Action           string `json:"action"`
	Reason           string `json:"reason"`
	RequestID        string `json:"request_id"`
}

type ModerationActionRequest struct {
	Reason    string `json:"reason"`
	ReportID  uint64 `json:"report_id"`
	RequestID string `json:"request_id"`
}

type ReportResponse struct {
	ID               uint64     `json:"id"`
	ReporterID       uint64     `json:"reporter_id"`
	TargetType       string     `json:"target_type"`
	TargetID         uint64     `json:"target_id"`
	ReasonCode       string     `json:"reason_code"`
	Detail           string     `json:"detail"`
	Status           string     `json:"status"`
	AssignedTo       *uint64    `json:"assigned_to,omitempty"`
	ResolutionReason *string    `json:"resolution_reason,omitempty"`
	ActionID         *uint64    `json:"action_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
}

type ActionResponse struct {
	ID          uint64    `json:"id"`
	ActorID     uint64    `json:"actor_id"`
	ReportID    *uint64   `json:"report_id,omitempty"`
	TargetType  string    `json:"target_type"`
	TargetID    uint64    `json:"target_id"`
	Action      string    `json:"action"`
	Reason      string    `json:"reason"`
	BeforeState string    `json:"before_state"`
	AfterState  string    `json:"after_state"`
	RequestID   string    `json:"request_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type ReportListResponse struct {
	Items    []ReportResponse `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    int64            `json:"total"`
}

type ActionListResponse struct {
	Items    []ActionResponse `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    int64            `json:"total"`
}

func toReportResponse(r Report) ReportResponse {
	return ReportResponse{ID: r.ID, ReporterID: r.ReporterID, TargetType: r.TargetType, TargetID: r.TargetID, ReasonCode: r.ReasonCode, Detail: r.Detail, Status: r.Status, AssignedTo: r.AssignedTo, ResolutionReason: r.ResolutionReason, ActionID: r.ActionID, CreatedAt: r.CreatedAt, ClosedAt: r.ClosedAt}
}

func toActionResponse(a ModerationAction) ActionResponse {
	return ActionResponse{ID: a.ID, ActorID: a.ActorID, ReportID: a.ReportID, TargetType: a.TargetType, TargetID: a.TargetID, Action: a.Action, Reason: a.Reason, BeforeState: a.BeforeState, AfterState: a.AfterState, RequestID: a.RequestID, CreatedAt: a.CreatedAt}
}
