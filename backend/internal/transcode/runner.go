package transcode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Runner interface {
	Process(ctx context.Context, inputPath, outputDir string, info MediaInfo, profiles []Profile) error
}

type FFmpegRunner struct {
	Path string
}

func (r FFmpegRunner) Process(ctx context.Context, inputPath, outputDir string, info MediaInfo, profiles []Profile) error {
	if len(profiles) == 0 {
		return fmt.Errorf("no transcode profiles selected")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create transcode output directory: %w", err)
	}
	coverAt := min(float64(info.DurationMS)/10000, 5)
	if coverAt < 0.1 {
		coverAt = 0.1
	}
	coverPath := filepath.Join(outputDir, "cover.jpg")
	if err := r.run(ctx,
		"-y", "-ss", strconv.FormatFloat(coverAt, 'f', 3, 64), "-i", inputPath,
		"-frames:v", "1", "-vf", "scale='min(1280,iw)':-2", "-q:v", "3", coverPath,
	); err != nil {
		return fmt.Errorf("generate cover: %w", err)
	}

	for _, profile := range profiles {
		profileDir := filepath.Join(outputDir, profile.Name)
		if err := os.MkdirAll(profileDir, 0o755); err != nil {
			return fmt.Errorf("create profile directory: %w", err)
		}
		playlistPath := filepath.Join(profileDir, "index.m3u8")
		segmentPattern := filepath.Join(profileDir, "segment-%05d.ts")
		args := []string{
			"-y", "-i", inputPath,
			"-map", "0:v:0", "-map", "0:a:0?",
			"-vf", fmt.Sprintf("scale=%d:%d", profile.Width, profile.Height),
			"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "main",
			"-b:v", profile.VideoBitrate, "-maxrate", profile.VideoBitrate,
			"-bufsize", "2M", "-pix_fmt", "yuv420p",
			"-sc_threshold", "0", "-force_key_frames", "expr:gte(t,n_forced*6)",
		}
		if info.HasAudio {
			args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "2")
		} else {
			args = append(args, "-an")
		}
		args = append(args,
			"-f", "hls", "-hls_time", "6", "-hls_playlist_type", "vod",
			"-hls_flags", "independent_segments", "-hls_segment_filename", segmentPattern,
			playlistPath,
		)
		if err := r.run(ctx, args...); err != nil {
			return fmt.Errorf("transcode %s: %w", profile.Name, err)
		}
	}

	if err := os.WriteFile(filepath.Join(outputDir, "master.m3u8"), []byte(MasterPlaylist(profiles)), 0o644); err != nil {
		return fmt.Errorf("write master playlist: %w", err)
	}
	return nil
}

func (r FFmpegRunner) run(ctx context.Context, args ...string) error {
	output, err := exec.CommandContext(ctx, r.Path, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if len(message) > 2000 {
		message = message[len(message)-2000:]
	}
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}

func MasterPlaylist(profiles []Profile) string {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	for _, profile := range profiles {
		fmt.Fprintf(&builder, "#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n%s/index.m3u8\n", profile.Bandwidth, profile.Width, profile.Height, profile.Name)
	}
	return builder.String()
}
