package transcode

import "testing"

func TestParseProbeJSON(t *testing.T) {
	input := []byte(`{
      "streams": [
        {"codec_type":"video","width":1920,"height":1080,"duration":"12.345"},
        {"codec_type":"audio"}
      ],
      "format":{"duration":"12.400"}
    }`)
	info, err := ParseProbeJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 1920 || info.Height != 1080 || info.DurationMS != 12400 || !info.HasAudio {
		t.Fatalf("info = %+v", info)
	}
}

func TestParseProbeJSONRejectsMissingVideo(t *testing.T) {
	if _, err := ParseProbeJSON([]byte(`{"streams":[{"codec_type":"audio"}],"format":{"duration":"1"}}`)); err == nil {
		t.Fatal("expected missing video stream error")
	}
}
