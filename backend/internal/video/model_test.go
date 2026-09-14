package video

import "testing"

func TestProcessingStatusString(t *testing.T) {
	if got := StatusProcessing.String(); got != "processing" {
		t.Fatalf("StatusProcessing.String() = %q, want processing", got)
	}
}
