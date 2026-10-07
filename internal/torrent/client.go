package torrent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// ClientConfig holds configuration for the BitTorrent client.
type ClientConfig struct {
	DataDir         string
	ListenPort      int
	DisableUpload   bool
	DisableDHT      bool
	DisableTrackers bool
}

// DefaultClientConfig returns standard settings for IDMM.
func DefaultClientConfig(dataDir string) ClientConfig {
	if dataDir == "" {
		dataDir = "./downloads"
	}
	return ClientConfig{
		DataDir:         dataDir,
		ListenPort:      0, // random available port
		DisableUpload:   false,
		DisableDHT:      false,
		DisableTrackers: false,
	}
}

// Client wraps the BitTorrent client engine.
type Client struct {
	cfg       ClientConfig
	inner     *torrent.Client
	mu        sync.RWMutex
	downloads map[string]*Download
	isClosed  bool
}

// NewClient initializes a new BitTorrent engine client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	tcfg := torrent.NewDefaultClientConfig()
	tcfg.DataDir = cfg.DataDir
	tcfg.NoUpload = cfg.DisableUpload
	tcfg.NoDHT = cfg.DisableDHT
	tcfg.DisableTrackers = cfg.DisableTrackers
	if cfg.ListenPort > 0 {
		tcfg.ListenPort = cfg.ListenPort
	}

	tc, err := torrent.NewClient(tcfg)
	if err != nil {
		return nil, fmt.Errorf("failed to start torrent client: %w", err)
	}

	return &Client{
		cfg:       cfg,
		inner:     tc,
		downloads: make(map[string]*Download),
	}, nil
}

// AddInput dynamically resolves input (magnet URI, torrent file path, or HTTP URL to .torrent).
func (c *Client) AddInput(ctx context.Context, input string) (*Download, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "magnet:?") {
		return c.AddMagnet(ctx, input)
	}

	// If it's a web URL pointing to a .torrent file
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return c.AddTorrentURL(ctx, input)
	}

	// Local .torrent file
	return c.AddTorrentFile(ctx, input)
}

// AddMagnet adds a torrent via magnet link.
func (c *Client) AddMagnet(ctx context.Context, magnetURI string) (*Download, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isClosed {
		return nil, fmt.Errorf("torrent client is closed")
	}

	mag, err := metainfo.ParseMagnetUri(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("invalid magnet link: %w", err)
	}
	if mag.InfoHash == (metainfo.Hash{}) {
		return nil, fmt.Errorf("magnet link has no infohash")
	}

	t, err := c.inner.AddMagnet(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed adding magnet: %w", err)
	}

	dl := newDownload(t, c.cfg.DataDir)
	c.downloads[t.InfoHash().HexString()] = dl
	return dl, nil
}

// AddTorrentFile adds a torrent from a local .torrent file path.
func (c *Client) AddTorrentFile(ctx context.Context, filePath string) (*Download, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isClosed {
		return nil, fmt.Errorf("torrent client is closed")
	}

	mi, err := metainfo.LoadFromFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed reading .torrent file: %w", err)
	}

	t, err := c.inner.AddTorrent(mi)
	if err != nil {
		return nil, fmt.Errorf("failed adding torrent: %w", err)
	}

	dl := newDownload(t, c.cfg.DataDir)
	c.downloads[t.InfoHash().HexString()] = dl
	return dl, nil
}

// AddTorrentURL downloads a remote .torrent file into memory/temp and adds it.
func (c *Client) AddTorrentURL(ctx context.Context, url string) (*Download, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch .torrent url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d when fetching .torrent", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading .torrent payload: %w", err)
	}

	return c.AddTorrentBytes(ctx, data)
}

// AddTorrentBytes adds a torrent from in-memory byte slice.
func (c *Client) AddTorrentBytes(ctx context.Context, data []byte) (*Download, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isClosed {
		return nil, fmt.Errorf("torrent client is closed")
	}

	mi, err := metainfo.Load(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid .torrent data: %w", err)
	}

	t, err := c.inner.AddTorrent(mi)
	if err != nil {
		return nil, fmt.Errorf("failed adding torrent: %w", err)
	}

	dl := newDownload(t, c.cfg.DataDir)
	c.downloads[t.InfoHash().HexString()] = dl
	return dl, nil
}

// GetDownload retrieves a download by its infohash hex string.
func (c *Client) GetDownload(infoHash string) (*Download, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	dl, ok := c.downloads[strings.ToLower(infoHash)]
	return dl, ok
}

// ListDownloads returns all active torrent downloads.
func (c *Client) ListDownloads() []*Download {
	c.mu.RLock()
	defer c.mu.RUnlock()
	list := make([]*Download, 0, len(c.downloads))
	for _, dl := range c.downloads {
		list = append(list, dl)
	}
	return list
}

// Close gracefully terminates the torrent client and all connections.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return nil
	}
	c.isClosed = true
	c.inner.Close()
	return nil
}
