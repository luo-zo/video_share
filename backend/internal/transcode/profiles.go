package transcode

import (
	"fmt"
	"math"
)

type Profile struct {
	Name         string
	Width        int
	Height       int
	VideoBitrate string
	Bandwidth    int
}

func SelectProfiles(info MediaInfo) []Profile {
	targets := []struct {
		height    int
		bitrate   string
		bandwidth int
	}{
		{360, "800k", 928000},
		{720, "2500k", 2628000},
		{1080, "5000k", 5128000},
	}
	profiles := make([]Profile, 0, len(targets))
	for _, target := range targets {
		if target.height > info.Height {
			continue
		}
		profiles = append(profiles, Profile{
			Name:         fmt.Sprintf("%dp", target.height),
			Width:        scaledEvenWidth(info.Width, info.Height, target.height),
			Height:       target.height,
			VideoBitrate: target.bitrate,
			Bandwidth:    target.bandwidth,
		})
	}
	if len(profiles) > 0 {
		return profiles
	}
	height := info.Height - info.Height%2
	width := info.Width - info.Width%2
	if height < 2 {
		height = 2
	}
	if width < 2 {
		width = 2
	}
	return []Profile{{
		Name:         fmt.Sprintf("%dp", height),
		Width:        width,
		Height:       height,
		VideoBitrate: "500k",
		Bandwidth:    628000,
	}}
}

func scaledEvenWidth(sourceWidth, sourceHeight, targetHeight int) int {
	width := int(math.Round((float64(sourceWidth)*float64(targetHeight)/float64(sourceHeight))/2) * 2)
	if width > sourceWidth {
		width = sourceWidth - sourceWidth%2
	}
	return max(width, 2)
}
