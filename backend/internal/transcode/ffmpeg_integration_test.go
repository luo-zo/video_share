//go:build integration

package transcode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealFFmpegProducesPlayableHLSFiles(t *testing.T) {
	ffmpeg := strings.TrimSpace(os.Getenv("TEST_FFMPEG_PATH"))
	ffprobe := strings.TrimSpace(os.Getenv("TEST_FFPROBE_PATH"))
	if ffmpeg == "" || ffprobe == "" {
		if strings.EqualFold(os.Getenv("REQUIRE_INTEGRATION_TESTS"), "true") {
			t.Fatal("TEST_FFMPEG_PATH and TEST_FFPROBE_PATH are required")
		}
		t.Skip("FFmpeg integration paths not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	workDir := t.TempDir()
	input := filepath.Join(workDir, "sample.mp4")
	output, err := exec.CommandContext(ctx, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-t", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", input,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("generate synthetic sample: %v: %s", err, output)
	}
	info, err := (FFprobe{Path: ffprobe}).Probe(ctx, input)
	if err != nil || info.Width != 320 || info.Height != 180 || info.DurationMS < 1500 {
		t.Fatalf("ffprobe info=%+v err=%v", info, err)
	}
	profiles := SelectProfiles(info)
	resultDir := filepath.Join(workDir, "hls")
	if err := (FFmpegRunner{Path: ffmpeg}).Process(ctx, input, resultDir, info, profiles); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(resultDir, "cover.jpg"),
		filepath.Join(resultDir, "master.m3u8"),
		filepath.Join(resultDir, profiles[0].Name, "index.m3u8"),
		filepath.Join(resultDir, profiles[0].Name, "segment-00000.ts"),
	} {
		stat, err := os.Stat(path)
		if err != nil || stat.Size() == 0 {
			t.Fatalf("missing or empty transcode output %s: %v", path, err)
		}
	}
}
