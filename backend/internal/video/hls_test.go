package video

import (
	"strings"
	"testing"
)

func TestRewriteHLSManifestRewritesRelativeURIs(t *testing.T) {
	manifest := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=928000\n360p/index.m3u8\n"
	result, err := rewriteHLSManifest(42, "master.m3u8", []byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "/api/v1/videos/42/hls/360p/index.m3u8") {
		t.Fatalf("rewritten manifest = %s", result)
	}
}

func TestRewriteHLSManifestRejectsTraversal(t *testing.T) {
	if _, err := rewriteHLSManifest(42, "360p/index.m3u8", []byte("#EXTM3U\n../../secret.ts\n")); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	for _, requested := range []string{"../master.m3u8", "/master.m3u8", `..\master.m3u8`} {
		if _, err := safeHLSRelativePath(requested); err == nil {
			t.Fatalf("path %q should be rejected", requested)
		}
	}
}
