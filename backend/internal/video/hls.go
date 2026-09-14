package video

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var (
	ErrInvalidMediaPath = errors.New("invalid media path")
	manifestURI         = regexp.MustCompile(`URI="([^"]+)"`)
)

func safeHLSRelativePath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, `\`) || strings.HasPrefix(value, "/") {
		return "", ErrInvalidMediaPath
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrInvalidMediaPath
	}
	return cleaned, nil
}

func rewriteHLSManifest(videoID uint64, manifestPath string, data []byte) ([]byte, error) {
	current, err := safeHLSRelativePath(manifestPath)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			var rewriteErr error
			line = manifestURI.ReplaceAllStringFunc(line, func(match string) string {
				parts := manifestURI.FindStringSubmatch(match)
				if len(parts) != 2 {
					return match
				}
				rewritten, err := resolveManifestURI(videoID, current, parts[1])
				if err != nil {
					rewriteErr = err
					return match
				}
				return `URI="` + rewritten + `"`
			})
			if rewriteErr != nil {
				return nil, rewriteErr
			}
		} else if strings.TrimSpace(line) != "" {
			line, err = resolveManifestURI(videoID, current, strings.TrimSpace(line))
			if err != nil {
				return nil, err
			}
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan HLS manifest: %w", err)
	}
	return output.Bytes(), nil
}

func resolveManifestURI(videoID uint64, currentManifest, rawURI string) (string, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" {
		return "", ErrInvalidMediaPath
	}
	relative, err := safeHLSRelativePath(path.Join(path.Dir(currentManifest), parsed.Path))
	if err != nil {
		return "", err
	}
	segments := strings.Split(relative, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	result := fmt.Sprintf("/api/v1/videos/%d/hls/%s", videoID, strings.Join(segments, "/"))
	if parsed.RawQuery != "" {
		result += "?" + parsed.RawQuery
	}
	return result, nil
}
