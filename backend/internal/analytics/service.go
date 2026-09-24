package analytics

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

type ServiceOption func(*Service)

func WithClock(now func() time.Time) ServiceOption {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

func NewService(repo Repository, options ...ServiceOption) *Service {
	service := &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) StartSession(ctx context.Context, userID, videoID uint64) (StartResponse, error) {
	if userID == 0 {
		return StartResponse{}, ErrUnauthorized
	}
	result, err := s.repo.StartSession(ctx, userID, videoID, s.now().UTC())
	if err != nil {
		return StartResponse{}, err
	}
	return StartResponse{SessionID: result.Session.ID, VideoID: result.Session.VideoID, DurationMS: result.DurationMS, ResumePositionMS: result.ResumeMS, ExpiresAt: result.Session.ExpiresAt}, nil
}

func (s *Service) Heartbeat(ctx context.Context, userID uint64, sessionID string, input HeartbeatInput) (HeartbeatResponse, error) {
	if userID == 0 {
		return HeartbeatResponse{}, ErrUnauthorized
	}
	if strings.TrimSpace(sessionID) == "" {
		return HeartbeatResponse{}, ErrHeartbeatInvalid
	}
	result, err := s.repo.Heartbeat(ctx, userID, sessionID, input, s.now().UTC())
	if err != nil {
		return HeartbeatResponse{}, err
	}
	return HeartbeatResponse{SessionID: result.Session.ID, Seq: result.Session.LastSeq, PositionMS: result.Session.LastPositionMS, AcceptedDeltaMS: result.AcceptedDeltaMS, SessionCreditedMS: result.Session.CreditedMS, EffectiveWatchMS: result.EffectiveWatchMS, Qualified: result.Session.QualifiedAt != nil, Completed: result.Session.CompletedAt != nil}, nil
}

func (s *Service) CreatorSummary(ctx context.Context, creatorID uint64, days int) (CreatorSummaryResponse, error) {
	if creatorID == 0 {
		return CreatorSummaryResponse{}, ErrUnauthorized
	}
	return s.repo.CreatorSummary(ctx, creatorID, days, s.now().UTC())
}
