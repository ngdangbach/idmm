package media

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HLSSegment represents a single video/audio segment from an m3u8 playlist.
type HLSSegment struct {
	Index    int
	URL      string
	Duration float64
	KeyURL   string
	KeyData  []byte
	IV       []byte
}

// HLSProgress emits real-time statistics for HLS tasks.
type HLSProgress struct {
	TotalSegments      int     `json:"total_segments"`
	DownloadedSegments int     `json:"downloaded_segments"`
	ProgressPercent    float64 `json:"progress_percent"`
	SpeedBytesPerSec   int64   `json:"speed_bytes_sec"`
	Status             string  `json:"status"`
}

// HLSDownloader manages downloading and assembling m3u8 streams.
type HLSDownloader struct {
	mu           sync.Mutex
	playlistURL  string
	targetPath   string
	connections  int
	client       *http.Client
	progressChan chan HLSProgress
	status       string
}

// NewHLSDownloader creates a new HLS downloader.
func NewHLSDownloader(playlistURL, targetPath string, connections int) *HLSDownloader {
	if connections <= 0 {
		connections = 8
	}
	return &HLSDownloader{
		playlistURL:  playlistURL,
		targetPath:   targetPath,
		connections:  connections,
		client:       &http.Client{Timeout: 30 * time.Second},
		progressChan: make(chan HLSProgress, 100),
		status:       "PENDING",
	}
}

// ProgressChannel returns the progress channel.
func (h *HLSDownloader) ProgressChannel() <-chan HLSProgress {
	return h.progressChan
}

// Start begins the HLS stream download.
func (h *HLSDownloader) Start(ctx context.Context) error {
	h.mu.Lock()
	h.status = "DOWNLOADING"
	h.mu.Unlock()

	// 1. Fetch and resolve media playlist
	mediaURL, segments, err := h.parsePlaylist(ctx, h.playlistURL)
	if err != nil {
		return fmt.Errorf("failed to parse HLS playlist: %w", err)
	}

	if len(segments) == 0 {
		return fmt.Errorf("no media segments found in playlist %s", mediaURL)
	}

	// Ensure destination directory exists
	if dir := filepath.Dir(h.targetPath); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}

	out, err := os.Create(h.targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer out.Close()

	totalSegments := len(segments)

	// Fetch keys if encrypted
	keyCache := make(map[string][]byte)
	for i := range segments {
		if segments[i].KeyURL != "" && len(segments[i].KeyData) == 0 {
			key, ok := keyCache[segments[i].KeyURL]
			if !ok {
				key, err = h.fetchKey(ctx, segments[i].KeyURL)
				if err != nil {
					return fmt.Errorf("failed to fetch encryption key: %w", err)
				}
				keyCache[segments[i].KeyURL] = key
			}
			segments[i].KeyData = key
		}
	}

	// Download queue channels
	type downloadResult struct {
		Index int
		Data  []byte
		Err   error
	}

	jobs := make(chan *HLSSegment, totalSegments)
	results := make(chan downloadResult, totalSegments)

	// Launch worker pool
	var wg sync.WaitGroup
	workers := h.connections
	if workers > totalSegments {
		workers = totalSegments
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seg := range jobs {
				data, err := h.downloadAndDecryptSegment(ctx, seg)
				results <- downloadResult{
					Index: seg.Index,
					Data:  data,
					Err:   err,
				}
			}
		}()
	}

	for i := range segments {
		jobs <- &segments[i]
	}
	close(jobs)

	// Asynchronous collector to write segments to disk in strict numerical order
	buffer := make(map[int][]byte)
	nextToWrite := 0
	downloadedCount := 0

	var lastBytes int64 = 0
	lastTime := time.Now()
	var currentSpeed int64 = 0

	for downloadedCount < totalSegments {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-results:
			if res.Err != nil {
				return fmt.Errorf("error downloading segment %d: %w", res.Index, res.Err)
			}
			buffer[res.Index] = res.Data
			downloadedCount++

			// Calculate speed
			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()
			if elapsed >= 0.5 {
				diff := int64(len(res.Data))
				currentSpeed = int64(float64(diff) / elapsed)
				lastTime = now
				lastBytes = 0
			} else {
				lastBytes += int64(len(res.Data))
			}

			// Write any ready contiguous segments to disk
			for {
				data, exists := buffer[nextToWrite]
				if !exists {
					break
				}
				if _, err := out.Write(data); err != nil {
					return fmt.Errorf("failed writing to output file: %w", err)
				}
				delete(buffer, nextToWrite)
				nextToWrite++
			}

			// Report progress
			pct := (float64(downloadedCount) / float64(totalSegments)) * 100
			prog := HLSProgress{
				TotalSegments:      totalSegments,
				DownloadedSegments: downloadedCount,
				ProgressPercent:    pct,
				SpeedBytesPerSec:   currentSpeed,
				Status:             "DOWNLOADING",
			}
			select {
			case h.progressChan <- prog:
			default:
			}
		}
	}

	wg.Wait()

	h.mu.Lock()
	h.status = "COMPLETED"
	h.mu.Unlock()

	select {
	case h.progressChan <- HLSProgress{
		TotalSegments:      totalSegments,
		DownloadedSegments: totalSegments,
		ProgressPercent:    100,
		Status:             "COMPLETED",
	}:
	default:
	}

	return nil
}

func (h *HLSDownloader) parsePlaylist(ctx context.Context, plURL string) (string, []HLSSegment, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", plURL, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("playlist request returned status %d", resp.StatusCode)
	}

	baseURL, err := url.Parse(plURL)
	if err != nil {
		return "", nil, err
	}

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	isMaster := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			isMaster = true
		}
		lines = append(lines, line)
	}

	// Handle Master Playlist: Pick variant with highest bandwidth
	if isMaster {
		var bestURI string
		var maxBandwidth int64 = -1

		for i := 0; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "#EXT-X-STREAM-INF") && i+1 < len(lines) {
				bandwidth := parseAttributeInt(lines[i], "BANDWIDTH")
				uri := lines[i+1]
				if bandwidth > maxBandwidth {
					maxBandwidth = bandwidth
					bestURI = uri
				}
			}
		}

		if bestURI != "" {
			resolved, err := resolveURL(baseURL, bestURI)
			if err != nil {
				return "", nil, err
			}
			return h.parsePlaylist(ctx, resolved)
		}
	}

	// Handle Media Playlist
	var segments []HLSSegment
	var currentKeyURL string
	var currentIV []byte
	var currentDuration float64 = 0
	segIndex := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-KEY:") {
			method := parseAttributeString(line, "METHOD")
			if method == "AES-128" {
				keyURI := parseAttributeString(line, "URI")
				resolvedKeyURI, err := resolveURL(baseURL, keyURI)
				if err == nil {
					currentKeyURL = resolvedKeyURI
				}
				ivHex := parseAttributeString(line, "IV")
				if ivHex != "" {
					ivHex = strings.TrimPrefix(ivHex, "0x")
					currentIV, _ = hex.DecodeString(ivHex)
				}
			}
		} else if strings.HasPrefix(line, "#EXTINF:") {
			durStr := strings.TrimPrefix(line, "#EXTINF:")
			durParts := strings.Split(durStr, ",")
			currentDuration, _ = strconv.ParseFloat(durParts[0], 64)
		} else if !strings.HasPrefix(line, "#") {
			segURL, err := resolveURL(baseURL, line)
			if err == nil {
				iv := currentIV
				if len(iv) == 0 {
					// Fallback: sequence number as 16-byte big endian IV
					iv = make([]byte, 16)
					binary.BigEndian.PutUint64(iv[8:], uint64(segIndex))
				}
				segments = append(segments, HLSSegment{
					Index:    segIndex,
					URL:      segURL,
					Duration: currentDuration,
					KeyURL:   currentKeyURL,
					IV:       iv,
				})
				segIndex++
			}
		}
	}

	return plURL, segments, nil
}

func (h *HLSDownloader) downloadAndDecryptSegment(ctx context.Context, seg *HLSSegment) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", seg.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("segment returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Decrypt if AES-128
	if len(seg.KeyData) > 0 {
		block, err := aes.NewCipher(seg.KeyData)
		if err != nil {
			return nil, fmt.Errorf("aes cipher failed: %w", err)
		}
		if len(data)%aes.BlockSize != 0 {
			return nil, fmt.Errorf("ciphertext is not multiple of block size")
		}
		mode := cipher.NewCBCDecrypter(block, seg.IV)
		mode.CryptBlocks(data, data)

		// Strip PKCS7 padding
		if len(data) > 0 {
			padLen := int(data[len(data)-1])
			if padLen <= aes.BlockSize && padLen <= len(data) {
				data = data[:len(data)-padLen]
			}
		}
	}

	return data, nil
}

func (h *HLSDownloader) fetchKey(ctx context.Context, keyURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", keyURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func resolveURL(base *url.URL, relative string) (string, error) {
	u, err := url.Parse(relative)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(u).String(), nil
}

func parseAttributeString(line, key string) string {
	idx := strings.Index(line, key+"=")
	if idx == -1 {
		return ""
	}
	sub := line[idx+len(key)+1:]
	if strings.HasPrefix(sub, `"`) {
		end := strings.Index(sub[1:], `"`)
		if end != -1 {
			return sub[1 : end+1]
		}
	}
	end := strings.IndexAny(sub, ", \r\n")
	if end != -1 {
		return sub[:end]
	}
	return sub
}

func parseAttributeInt(line, key string) int64 {
	valStr := parseAttributeString(line, key)
	val, _ := strconv.ParseInt(valStr, 10, 64)
	return val
}
