package torrent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClientLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "idmm_torrent_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := DefaultClientConfig(tmpDir)
	cfg.DisableDHT = true
	cfg.DisableTrackers = true

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	if client.inner == nil {
		t.Fatal("expected inner torrent client to be initialized")
	}

	// Test adding a valid magnet URI structure
	magnetURI := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=Ubuntu+ISO"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dl, err := client.AddInput(ctx, magnetURI)
	if err != nil {
		t.Fatalf("AddInput with magnet failed: %v", err)
	}

	if dl.InfoHash() != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("unexpected infohash: %s", dl.InfoHash())
	}

	// Verify retrieval from client
	retrieved, ok := client.GetDownload(dl.InfoHash())
	if !ok || retrieved == nil {
		t.Fatal("expected download to be retrievable by infohash")
	}

	list := client.ListDownloads()
	if len(list) != 1 {
		t.Errorf("expected 1 download in list, got %d", len(list))
	}

	stats := dl.Stats()
	if stats.Status != StatusFetchingMetadata {
		t.Errorf("expected status to be METADATA, got %s", stats.Status)
	}
	if stats.InfoHash != dl.InfoHash() {
		t.Errorf("expected matching infohash in stats")
	}
}

func TestClientInvalidInputs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "idmm_torrent_test_err_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := DefaultClientConfig(tmpDir)
	cfg.DisableDHT = true
	cfg.DisableTrackers = true

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test invalid magnet URI
	_, err = client.AddMagnet(ctx, "magnet:?invalid")
	if err == nil {
		t.Error("expected error for invalid magnet link")
	}

	// Test non-existent file
	_, err = client.AddTorrentFile(ctx, filepath.Join(tmpDir, "non_existent.torrent"))
	if err == nil {
		t.Error("expected error for non-existent torrent file")
	}

	// Test corrupt byte slice
	_, err = client.AddTorrentBytes(ctx, []byte("not a bencoded torrent file"))
	if err == nil {
		t.Error("expected error for invalid torrent byte payload")
	}
}
