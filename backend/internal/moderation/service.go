package moderation

import (
	"context"
	"errors"
)

var ErrPaginationInvalid = errors.New("invalid pagination")

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreateReport(ctx context.Context, reporterID uint64, input CreateReportRequest) (ReportResponse, error) {
	row, err := s.repo.CreateReport(ctx, reporterID, input)
	if err != nil {
		return ReportResponse{}, err
	}
	return toReportResponse(*row), nil
}

func (s *Service) ListMyReports(ctx context.Context, reporterID uint64, page, pageSize int) (ReportListResponse, error) {
	return s.listReports(ctx, reporterID, false, page, pageSize)
}

func (s *Service) ListAdminReports(ctx context.Context, page, pageSize int) (ReportListResponse, error) {
	return s.listReports(ctx, 0, true, page, pageSize)
}

func (s *Service) listReports(ctx context.Context, reporterID uint64, admin bool, page, pageSize int) (ReportListResponse, error) {
	if page < 1 || pageSize < 1 || pageSize > 50 {
		return ReportListResponse{}, ErrPaginationInvalid
	}
	rows, total, err := s.repo.ListReports(ctx, reporterID, admin, page, pageSize)
	if err != nil {
		return ReportListResponse{}, err
	}
	items := make([]ReportResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toReportResponse(row))
	}
	return ReportListResponse{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) AssignReport(ctx context.Context, adminID, reportID uint64, requestID string) (ReportResponse, error) {
	row, err := s.repo.AssignReport(ctx, adminID, reportID, requestID)
	if err != nil {
		return ReportResponse{}, err
	}
	return toReportResponse(*row), nil
}

func (s *Service) ResolveReport(ctx context.Context, adminID, reportID uint64, input ReportDecisionRequest) (ReportResponse, error) {
	row, err := s.repo.ResolveReport(ctx, adminID, reportID, input)
	if err != nil {
		return ReportResponse{}, err
	}
	return toReportResponse(*row), nil
}

func (s *Service) ModerateTarget(ctx context.Context, adminID uint64, targetType string, targetID uint64, action string, input ModerationActionRequest) (ActionResponse, error) {
	row, err := s.repo.ModerateTarget(ctx, adminID, targetType, targetID, action, input)
	if err != nil {
		return ActionResponse{}, err
	}
	return toActionResponse(*row), nil
}

func (s *Service) ListActions(ctx context.Context, page, pageSize int) (ActionListResponse, error) {
	if page < 1 || pageSize < 1 || pageSize > 50 {
		return ActionListResponse{}, ErrPaginationInvalid
	}
	rows, total, err := s.repo.ListActions(ctx, page, pageSize)
	if err != nil {
		return ActionListResponse{}, err
	}
	items := make([]ActionResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toActionResponse(row))
	}
	return ActionListResponse{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}
