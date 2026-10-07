package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHLSDownloaderParseAndAssemble(t *testing.T) {
	// Mock HLS server
	seg1Content := []byte("SEGMENT_1_DATA_PAYLOAD")
	seg2Content := []byte("SEGMENT_2_DATA_PAYLOAD")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			playlist := strings.Join([]string{
				"#EXTM3U",
				"#EXT-X-VERSION:3",
				"#EXT-X-TARGETDURATION:10",
				"#EXT-X-MEDIA-SEQUENCE:0",
				"#EXTINF:5.0,",
				"seg1.ts",
				"#EXTINF:5.0,",
				"seg2.ts",
				"#EXT-X-ENDLIST",
			}, "\n")
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte(playlist))
		case "/seg1.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write(seg1Content)
		case "/seg2.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write(seg2Content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "output.ts")

	hlsDownloader := NewHLSDownloader(server.URL+"/playlist.m3u8", outputPath, 4)

	ctx := context.Background()
	err := hlsDownloader.Start(ctx)
	if err != nil {
		t.Fatalf("HLS download failed: %v", err)
	}

	// Verify assembled file
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	expected := append(seg1Content, seg2Content...)
	if string(data) != string(expected) {
		t.Fatalf("assembled file content mismatch: expected %q, got %q", expected, data)
	}
}
