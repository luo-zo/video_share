package transcode

import "testing"

func TestSelectProfilesNeverUpscales(t *testing.T) {
	profiles := SelectProfiles(MediaInfo{Width: 854, Height: 480})
	if len(profiles) != 1 || profiles[0].Height != 360 {
		t.Fatalf("profiles = %+v, want only 360p", profiles)
	}
	if profiles[0].Width%2 != 0 || profiles[0].Width > 854 {
		t.Fatalf("invalid scaled width: %+v", profiles[0])
	}
}

func TestSelectProfilesIncludesSourceSizedFallback(t *testing.T) {
	profiles := SelectProfiles(MediaInfo{Width: 426, Height: 240})
	if len(profiles) != 1 || profiles[0].Height != 240 || profiles[0].Width != 426 {
		t.Fatalf("profiles = %+v", profiles)
	}
}

func TestSelectProfilesProducesStandardLadder(t *testing.T) {
	profiles := SelectProfiles(MediaInfo{Width: 1920, Height: 1080})
	if len(profiles) != 3 || profiles[0].Height != 360 || profiles[1].Height != 720 || profiles[2].Height != 1080 {
		t.Fatalf("profiles = %+v", profiles)
	}
}
