package transcode

import (
	"strings"
	"testing"
)

func TestMasterPlaylistUsesRelativeVariantPaths(t *testing.T) {
	profiles := []Profile{{Name: "360p", Width: 640, Height: 360, Bandwidth: 928000}, {Name: "720p", Width: 1280, Height: 720, Bandwidth: 2628000}}
	playlist := MasterPlaylist(profiles)
	for _, want := range []string{"#EXTM3U", "RESOLUTION=640x360", "360p/index.m3u8", "RESOLUTION=1280x720", "720p/index.m3u8"} {
		if !strings.Contains(playlist, want) {
			t.Fatalf("playlist missing %q:\n%s", want, playlist)
		}
	}
}
