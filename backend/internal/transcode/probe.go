package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
)

type MediaInfo struct {
	DurationMS uint64
	Width      int
	Height     int
	HasAudio   bool
}

type Prober interface {
	Probe(ctx context.Context, inputPath string) (MediaInfo, error)
}

type FFprobe struct {
	Path string
}

func (p FFprobe) Probe(ctx context.Context, inputPath string) (MediaInfo, error) {
	output, err := exec.CommandContext(ctx, p.Path,
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		inputPath,
	).Output()
	if err != nil {
		return MediaInfo{}, fmt.Errorf("run ffprobe: %w", err)
	}
	return ParseProbeJSON(output)
}

func ParseProbeJSON(data []byte) (MediaInfo, error) {
	var document struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Duration  string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return MediaInfo{}, fmt.Errorf("decode ffprobe output: %w", err)
	}
	var info MediaInfo
	var streamDuration string
	for _, stream := range document.Streams {
		switch stream.CodecType {
		case "video":
			if info.Width == 0 {
				info.Width, info.Height = stream.Width, stream.Height
				streamDuration = stream.Duration
			}
		case "audio":
			info.HasAudio = true
		}
	}
	if info.Width <= 0 || info.Height <= 0 {
		return MediaInfo{}, fmt.Errorf("ffprobe found no usable video stream")
	}
	duration := document.Format.Duration
	if duration == "" {
		duration = streamDuration
	}
	seconds, err := strconv.ParseFloat(duration, 64)
	if err != nil || seconds <= 0 {
		return MediaInfo{}, fmt.Errorf("ffprobe returned invalid duration")
	}
	info.DurationMS = uint64(math.Round(seconds * 1000))
	return info, nil
}
