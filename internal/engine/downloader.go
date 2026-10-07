package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FileInfo holds metadata retrieved during the probe phase.
type FileInfo struct {
	URL          string
	Filename     string
	TotalSize    int64
	AcceptRanges bool
	ETag         string
	LastModified string
	ContentType  string
}

// Downloader coordinates the entire download task lifecycle.
type Downloader struct {
	mu           sync.Mutex
	dataCond     *sync.Cond
	config       TaskConfig
	fileInfo     FileInfo
	segmentMgr   *SegmentManager
	writer       *FileWriter
	limiter      *SpeedLimiter
	status       TaskStatus
	client       *http.Client
	progressChan chan ProgressUpdate

	cancel       context.CancelFunc
	ctx          context.Context
	lastBytes    int64
	lastTime     time.Time
	currentSpeed int64
}

// NewDownloader creates a new downloader instance.
func NewDownloader(cfg TaskConfig) *Downloader {
	if cfg.Connections <= 0 {
		cfg.Connections = 8
	}
	if cfg.MinSegmentSize <= 0 {
		cfg.MinSegmentSize = 512 * 1024
	}

	d := &Downloader{
		config:       cfg,
		segmentMgr:   NewSegmentManager(cfg.MinSegmentSize),
		limiter:      NewSpeedLimiter(cfg.MaxSpeedBytesPerSec),
		status:       StatusPending,
		progressChan: make(chan ProgressUpdate, 100),
		client: &http.Client{
			Timeout: 0,
		},
	}
	d.dataCond = sync.NewCond(&d.mu)
	return d
}

// ProgressChannel provides real-time progress updates.
func (d *Downloader) ProgressChannel() <-chan ProgressUpdate {
	return d.progressChan
}

// Status returns current task status.
func (d *Downloader) Status() TaskStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

// TargetPath returns the destination path.
func (d *Downloader) TargetPath() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.config.TargetPath
}

// TotalSize returns the file size if known.
func (d *Downloader) TotalSize() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fileInfo.TotalSize
}

// ContentType returns detected MIME type.
func (d *Downloader) ContentType() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fileInfo.ContentType
}

// Filename returns file name.
func (d *Downloader) Filename() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fileInfo.Filename
}

// AcceptRanges returns whether server supports byte range requests.
func (d *Downloader) AcceptRanges() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fileInfo.AcceptRanges
}

// Probe inspects headers of the target URL to extract metadata.
func (d *Downloader) Probe(ctx context.Context) (*FileInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", d.config.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range d.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent) {
		getReq, getErr := http.NewRequestWithContext(ctx, "GET", d.config.URL, nil)
		if getErr != nil {
			return nil, getErr
		}
		getReq.Header.Set("Range", "bytes=0-0")
		for k, v := range d.config.Headers {
			getReq.Header.Set(k, v)
		}
		resp, err = d.client.Do(getReq)
		if err != nil {
			return nil, fmt.Errorf("probe failed: %w", err)
		}
	}
	defer resp.Body.Close()

	info := FileInfo{
		URL:          d.config.URL,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		ContentType:  resp.Header.Get("Content-Type"),
	}

	if strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") || resp.StatusCode == http.StatusPartialContent {
		info.AcceptRanges = true
	}

	if cr := resp.Header.Get("Content-Range"); cr != "" {
		parts := strings.Split(cr, "/")
		if len(parts) == 2 {
			if size, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				info.TotalSize = size
				info.AcceptRanges = true
			}
		}
	}
	if info.TotalSize == 0 && resp.ContentLength > 0 {
		info.TotalSize = resp.ContentLength
	}

	cd := resp.Header.Get("Content-Disposition")
	if cd != "" && strings.Contains(cd, "filename=") {
		parts := strings.Split(cd, "filename=")
		if len(parts) > 1 {
			fn := strings.Trim(parts[1], ` "';`)
			info.Filename = fn
		}
	}
	if info.Filename == "" {
		if parsed, err := url.Parse(d.config.URL); err == nil {
			base := path.Base(parsed.Path)
			if base != "" && base != "/" && base != "." {
				info.Filename = base
			}
		}
	}
	if info.Filename == "" {
		info.Filename = "downloaded_file"
	}

	d.fileInfo = info
	return &d.fileInfo, nil
}

// Start begins downloading.
func (d *Downloader) Start(parentCtx context.Context) error {
	d.mu.Lock()
	if d.status == StatusDownloading {
		d.mu.Unlock()
		return errors.New("download already in progress")
	}
	d.ctx, d.cancel = context.WithCancel(parentCtx)
	d.status = StatusDownloading
	d.lastTime = time.Now()
	d.mu.Unlock()

	if d.fileInfo.TotalSize == 0 {
		if _, err := d.Probe(d.ctx); err != nil {
			d.fail(err)
			return err
		}
	}

	if d.config.TargetPath == "" {
		d.config.TargetPath = d.fileInfo.Filename
	} else {
		if fi, err := os.Stat(d.config.TargetPath); err == nil && fi.IsDir() {
			d.config.TargetPath = filepath.Join(d.config.TargetPath, d.fileInfo.Filename)
		}
	}

	resumed := false
	if d.fileInfo.AcceptRanges && d.fileInfo.TotalSize > 0 {
		if state, err := LoadState(d.config.TargetPath); err == nil {
			if state.TotalSize == d.fileInfo.TotalSize && len(state.Segments) > 0 {
				d.segmentMgr.RestoreSegments(state.Segments)
				resumed = true
			}
		}
	}

	if !resumed {
		if d.fileInfo.AcceptRanges && d.fileInfo.TotalSize > 0 {
			d.segmentMgr.InitializeSegments(d.fileInfo.TotalSize, d.config.Connections)
		} else {
			d.segmentMgr.InitializeSegments(d.fileInfo.TotalSize, 1)
		}
	}

	fw, err := OpenFileWriter(d.config.TargetPath, d.fileInfo.TotalSize)
	if err != nil {
		d.fail(err)
		return err
	}
	d.writer = fw

	progressDone := make(chan struct{})
	go d.monitorProgress(progressDone)

	var downloadErr error
	if d.fileInfo.AcceptRanges && d.fileInfo.TotalSize > 0 {
		downloadErr = d.downloadMultiThreaded()
	} else {
		downloadErr = d.downloadSingleThreaded()
	}

	close(progressDone)
	_ = d.writer.Close()

	if downloadErr != nil {
		if errors.Is(downloadErr, context.Canceled) {
			d.mu.Lock()
			d.status = StatusPaused
			d.mu.Unlock()
			d.saveCurrentState()
			d.emitProgress()
			return nil
		}
		d.fail(downloadErr)
		return downloadErr
	}

	RemoveState(d.config.TargetPath)
	d.mu.Lock()
	d.status = StatusCompleted
	d.dataCond.Broadcast()
	d.mu.Unlock()
	d.emitProgress()

	return nil
}

// downloadMultiThreaded manages concurrent workers with dynamic re-splitting.
func (d *Downloader) downloadMultiThreaded() error {
	var wg sync.WaitGroup
	workers := d.config.Connections
	errChan := make(chan error, workers)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-d.ctx.Done():
					return
				default:
				}

				seg := d.segmentMgr.GetPendingSegment()
				if seg == nil {
					seg = d.segmentMgr.TrySplitLargest()
				}

				if seg == nil {
					if d.segmentMgr.IsCompleted() {
						return
					}
					time.Sleep(100 * time.Millisecond)
					if d.segmentMgr.IsCompleted() {
						return
					}
					continue
				}

				if err := d.downloadSegment(seg); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					seg.Status = SegmentFailed
					select {
					case errChan <- err:
					default:
					}
					return
				}
			}
		}(w)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return <-errChan
	}
	if d.ctx.Err() != nil {
		return d.ctx.Err()
	}
	return nil
}

// downloadSegment downloads a specific byte range directly into the target file.
func (d *Downloader) downloadSegment(seg *Segment) error {
	for seg.Remaining() > 0 {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
		}

		currentOffset := seg.CurrentOffset()
		req, err := http.NewRequestWithContext(d.ctx, "GET", d.config.URL, nil)
		if err != nil {
			return err
		}
		rangeHeader := fmt.Sprintf("bytes=%d-%d", currentOffset, seg.End)
		req.Header.Set("Range", rangeHeader)
		for k, v := range d.config.Headers {
			req.Header.Set(k, v)
		}

		resp, err := d.client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("unexpected status %d for range %s", resp.StatusCode, rangeHeader)
		}

		reader := NewLimitedReader(d.ctx, resp.Body, d.limiter)
		buf := make([]byte, 64*1024)

		for {
			rem := seg.Remaining()
			if rem <= 0 && seg.End >= 0 {
				resp.Body.Close()
				break
			}
			toRead := len(buf)
			if seg.End >= 0 && int64(toRead) > rem {
				toRead = int(rem)
			}

			n, readErr := reader.Read(buf[:toRead])
			if n > 0 {
				writeOffset := seg.CurrentOffset()
				_, writeErr := d.writer.WriteAt(buf[:n], writeOffset)
				if writeErr != nil {
					resp.Body.Close()
					return fmt.Errorf("disk write failed at offset %d: %w", writeOffset, writeErr)
				}
				seg.Downloaded += int64(n)

				// Notify any stream readers waiting for bytes
				d.dataCond.Broadcast()
			}

			if readErr != nil {
				resp.Body.Close()
				if readErr == io.EOF {
					break
				}
				return readErr
			}
		}

		if seg.Remaining() <= 0 {
			seg.Status = SegmentCompleted
			d.dataCond.Broadcast()
			break
		}
	}
	return nil
}

// downloadSingleThreaded handles servers without Range support.
func (d *Downloader) downloadSingleThreaded() error {
	req, err := http.NewRequestWithContext(d.ctx, "GET", d.config.URL, nil)
	if err != nil {
		return err
	}
	for k, v := range d.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error %d", resp.StatusCode)
	}

	reader := NewLimitedReader(d.ctx, resp.Body, d.limiter)
	buf := make([]byte, 64*1024)

	segs := d.segmentMgr.GetAllSegments()
	var seg *Segment
	if len(segs) > 0 {
		seg = segs[0]
		seg.Status = SegmentDownloading
	}

	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
		}

		n, readErr := reader.Read(buf)
		if n > 0 {
			if _, writeErr := d.writer.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			if seg != nil {
				seg.Downloaded += int64(n)
			}
			d.dataCond.Broadcast()
		}
		if readErr != nil {
			if readErr == io.EOF {
				if seg != nil {
					seg.Status = SegmentCompleted
				}
				d.dataCond.Broadcast()
				break
			}
			return readErr
		}
	}
	return nil
}

// Prioritize requests downloading for a specific byte offset immediately.
func (d *Downloader) Prioritize(offset int64) {
	d.segmentMgr.PrioritizeOffset(offset)
}

// AvailableBytesAt returns how many contiguous bytes are ready to read from offset.
func (d *Downloader) AvailableBytesAt(offset int64) int64 {
	d.mu.Lock()
	st := d.status
	total := d.fileInfo.TotalSize
	d.mu.Unlock()

	if st == StatusCompleted && total > 0 && offset < total {
		return total - offset
	}
	return d.segmentMgr.AvailableBytesAt(offset)
}

// ReadAt reads downloaded bytes from target file.
func (d *Downloader) ReadAt(p []byte, offset int64) (int, error) {
	d.mu.Lock()
	targetPath := d.config.TargetPath
	writer := d.writer
	d.mu.Unlock()

	if writer != nil {
		n, err := writer.ReadAt(p, offset)
		if err == nil {
			return n, nil
		}
	}

	// Fallback to reading directly from file on disk if writer closed or completed
	f, err := os.Open(targetPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.ReadAt(p, offset)
}

// NewStreamReader creates an io.ReadCloser for media streaming with on-demand waiting.
func (d *Downloader) NewStreamReader(ctx context.Context, start, end int64) (io.ReadCloser, error) {
	if end < start && end != -1 {
		return nil, errors.New("invalid range")
	}
	if end == -1 {
		end = d.fileInfo.TotalSize - 1
	}

	d.Prioritize(start)

	return &StreamReader{
		d:       d,
		ctx:     ctx,
		currOff: start,
		endOff:  end,
	}, nil
}

// StreamReader enables sequential reading of live downloading files.
type StreamReader struct {
	d       *Downloader
	ctx     context.Context
	currOff int64
	endOff  int64
	closed  bool
}

func (sr *StreamReader) Read(p []byte) (int, error) {
	if sr.closed {
		return 0, io.ErrClosedPipe
	}
	if sr.currOff > sr.endOff {
		return 0, io.EOF
	}

	toRead := int64(len(p))
	remTotal := sr.endOff - sr.currOff + 1
	if toRead > remTotal {
		toRead = remTotal
	}

	// Wait for data to become available at sr.currOff
	for {
		select {
		case <-sr.ctx.Done():
			return 0, sr.ctx.Err()
		default:
		}

		avail := sr.d.AvailableBytesAt(sr.currOff)
		if avail > 0 {
			if int64(toRead) > avail {
				toRead = avail
			}
			n, err := sr.d.ReadAt(p[:toRead], sr.currOff)
			if n > 0 {
				sr.currOff += int64(n)
				return n, nil
			}
			if err != nil && !errors.Is(err, io.EOF) {
				return 0, err
			}
		}

		// Check if task completed or failed
		st := sr.d.Status()
		if st == StatusCompleted && sr.currOff >= sr.d.TotalSize() {
			return 0, io.EOF
		}
		if st == StatusFailed {
			return 0, errors.New("download failed")
		}

		// Request priority downloading near current offset
		sr.d.Prioritize(sr.currOff)

		// Wait briefly for new data chunk
		time.Sleep(50 * time.Millisecond)
	}
}

func (sr *StreamReader) Close() error {
	sr.closed = true
	return nil
}

func (d *Downloader) monitorProgress(done <-chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.updateMetrics()
			d.saveCurrentState()
			d.emitProgress()
		case <-done:
			d.updateMetrics()
			d.emitProgress()
			return
		}
	}
}

func (d *Downloader) updateMetrics() {
	d.mu.Lock()
	defer d.mu.Unlock()

	totalDownloaded := d.segmentMgr.TotalDownloaded()
	now := time.Now()
	elapsed := now.Sub(d.lastTime).Seconds()

	if elapsed > 0 {
		diff := totalDownloaded - d.lastBytes
		if diff < 0 {
			diff = 0
		}
		d.currentSpeed = int64(float64(diff) / elapsed)
		d.lastBytes = totalDownloaded
		d.lastTime = now
	}
}

func (d *Downloader) saveCurrentState() {
	if !d.fileInfo.AcceptRanges || d.fileInfo.TotalSize <= 0 {
		return
	}
	state := &TaskState{
		URL:          d.config.URL,
		TargetPath:   d.config.TargetPath,
		TotalSize:    d.fileInfo.TotalSize,
		AcceptRanges: d.fileInfo.AcceptRanges,
		ETag:         d.fileInfo.ETag,
		LastModified: d.fileInfo.LastModified,
		Segments:     d.segmentMgr.GetAllSegments(),
	}
	_ = SaveState(d.config.TargetPath, state)
}

func (d *Downloader) emitProgress() {
	totalDownloaded := d.segmentMgr.TotalDownloaded()
	totalSize := d.fileInfo.TotalSize

	var percent float64 = 0
	if totalSize > 0 {
		percent = (float64(totalDownloaded) / float64(totalSize)) * 100
		if percent > 100 {
			percent = 100
		}
	}

	var eta int64 = 0
	if d.currentSpeed > 0 && totalSize > 0 {
		remainingBytes := totalSize - totalDownloaded
		if remainingBytes > 0 {
			eta = remainingBytes / d.currentSpeed
		}
	}

	activeConns := 0
	for _, s := range d.segmentMgr.GetAllSegments() {
		if s.Status == SegmentDownloading {
			activeConns++
		}
	}

	update := ProgressUpdate{
		TotalSize:         totalSize,
		DownloadedBytes:   totalDownloaded,
		SpeedBytesPerSec:  d.currentSpeed,
		ETASeconds:        eta,
		ProgressPercent:   percent,
		ActiveConnections: activeConns,
		Segments:          d.segmentMgr.GetAllSegments(),
		Status:            d.status,
	}

	select {
	case d.progressChan <- update:
	default:
	}
}

func (d *Downloader) fail(err error) {
	d.mu.Lock()
	d.status = StatusFailed
	d.dataCond.Broadcast()
	d.mu.Unlock()

	update := ProgressUpdate{
		Status: StatusFailed,
		Error:  err.Error(),
	}
	select {
	case d.progressChan <- update:
	default:
	}
}

// Pause pauses the task.
func (d *Downloader) Pause() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
	}
	d.dataCond.Broadcast()
}
