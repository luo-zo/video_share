package database

import (
	"testing"

	"video_share/migrations"
)

func TestCommunityMigrationsEmbedded(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"000004_add_community_schema.sql",
		"000005_backfill_community_data.sql",
	} {
		if _, err := migrations.FS.ReadFile(name); err != nil {
			t.Errorf("migration %s is not embedded: %v", name, err)
		}
	}
}
