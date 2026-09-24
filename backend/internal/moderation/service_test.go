package moderation

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	report Report
	err    error
}

func (f *fakeRepository) CreateReport(context.Context, uint64, CreateReportRequest) (*Report, error) {
	return &f.report, f.err
}
func (f *fakeRepository) ListReports(context.Context, uint64, bool, int, int) ([]Report, int64, error) {
	return []Report{f.report}, 1, f.err
}
func (f *fakeRepository) AssignReport(context.Context, uint64, uint64, string) (*Report, error) {
	return &f.report, f.err
}
func (f *fakeRepository) ResolveReport(context.Context, uint64, uint64, ReportDecisionRequest) (*Report, error) {
	return &f.report, f.err
}
func (f *fakeRepository) ModerateTarget(context.Context, uint64, string, uint64, string, ModerationActionRequest) (*ModerationAction, error) {
	return &ModerationAction{ID: 9, TargetType: TargetVideo, TargetID: 4}, f.err
}
func (f *fakeRepository) ListActions(context.Context, int, int) ([]ModerationAction, int64, error) {
	return []ModerationAction{{ID: 9}}, 1, f.err
}

func TestServiceProjectsReportsAndRejectsInvalidPaging(t *testing.T) {
	repo := &fakeRepository{report: Report{ID: 3, ReporterID: 7, TargetType: TargetVideo, TargetID: 4, Status: ReportOpen, CreatedAt: time.Unix(1, 0)}}
	svc := NewService(repo)
	result, err := svc.ListMyReports(context.Background(), 7, 1, 20)
	if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := svc.ListAdminReports(context.Background(), 0, 51); !errors.Is(err, ErrPaginationInvalid) {
		t.Fatalf("paging err=%v", err)
	}
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	repo := &fakeRepository{err: errors.New("database down")}
	_, err := NewService(repo).CreateReport(context.Background(), 7, CreateReportRequest{})
	if err == nil || err.Error() != "database down" {
		t.Fatalf("err=%v", err)
	}
}
