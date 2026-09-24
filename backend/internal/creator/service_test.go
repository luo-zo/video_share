package creator

import (
	"context"
	"errors"
	"testing"
)

type profileRepository struct {
	profile *ProfileResponse
	err     error
}

func (r *profileRepository) FindPublicProfile(context.Context, uint64, uint64) (*ProfileResponse, error) {
	return r.profile, r.err
}

func TestProfilePassesViewerAndKeepsPublicProjection(t *testing.T) {
	repo := &profileRepository{profile: &ProfileResponse{ID: 9, Username: "creator", Nickname: "创作者", Following: true}}
	service := NewService(repo, nil, nil)
	profile, err := service.Profile(context.Background(), 9, 7)
	if err != nil || profile.ID != 9 || !profile.Following {
		t.Fatalf("Profile = %+v, %v", profile, err)
	}
}

func TestProfileRejectsMissingOrHiddenCreator(t *testing.T) {
	service := NewService(&profileRepository{err: ErrNotFound}, nil, nil)
	if _, err := service.Profile(context.Background(), 0, 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing creator error = %v", err)
	}
	if _, err := service.Profile(context.Background(), 9, 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hidden creator error = %v", err)
	}
}

func TestPaginationMatchesPublicPageContract(t *testing.T) {
	if err := validatePagination(1, 50); err != nil {
		t.Fatalf("max page size rejected: %v", err)
	}
	for _, values := range [][2]int{{0, 20}, {1, 0}, {1, 51}} {
		if err := validatePagination(values[0], values[1]); !errors.Is(err, ErrPaginationInvalid) {
			t.Fatalf("validatePagination(%v) = %v", values, err)
		}
	}
}
