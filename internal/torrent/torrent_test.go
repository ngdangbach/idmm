package torrent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// helper to create a valid .torrent file on disk from scratch
func createSampleTorrent(t *testing.T, dir string, fileName string, content []byte) (string, metainfo.Hash, *metainfo.MetaInfo) {
	t.Helper()
	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	info := metainfo.Info{
		PieceLength: 32 * 1024,
	}
	if err := info.BuildFromFilePath(filePath); err != nil {
		t.Fatalf("failed to build info from file path: %v", err)
	}

	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal info: %v", err)
	}

	mi := metainfo.MetaInfo{
		InfoBytes: infoBytes,
	}
	mi.SetDefaults()

	torrentPath := filepath.Join(dir, fileName+".torrent")
	tf, err := os.Create(torrentPath)
	if err != nil {
		t.Fatalf("failed to create torrent file: %v", err)
	}
	defer tf.Close()

	if err := mi.Write(tf); err != nil {
		t.Fatalf("failed to write metainfo: %v", err)
	}

	ih := mi.HashInfoBytes()
	return torrentPath, ih, &mi
}

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

func TestAddTorrentFile_Metadata(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "idmm_torrent_meta_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := bytes.Repeat([]byte("IDMM Fast Torrent Engine Block Verification "), 1024) // 45 KB
	torrentPath, ih, mi := createSampleTorrent(t, tmpDir, "sample_document.pdf", content)

	cfg := DefaultClientConfig(tmpDir)
	cfg.DisableDHT = true
	cfg.DisableTrackers = true

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Test AddTorrentFile
	dl, err := client.AddTorrentFile(ctx, torrentPath)
	if err != nil {
		t.Fatalf("AddTorrentFile failed: %v", err)
	}

	if dl.InfoHash() != ih.HexString() {
		t.Errorf("infohash mismatch: expected %s, got %s", ih.HexString(), dl.InfoHash())
	}

	if !dl.HasMetadata() {
		t.Error("expected metadata to be available immediately from .torrent file")
	}

	if dl.Name() != "sample_document.pdf" {
		t.Errorf("expected name 'sample_document.pdf', got '%s'", dl.Name())
	}

	if dl.TotalLength() != int64(len(content)) {
		t.Errorf("expected total length %d, got %d", len(content), dl.TotalLength())
	}

	files := dl.Files()
	if len(files) != 1 {
		t.Fatalf("expected 1 file in torrent, got %d", len(files))
	}
	if files[0].Path != "sample_document.pdf" {
		t.Errorf("expected file path 'sample_document.pdf', got '%s'", files[0].Path)
	}

	// 2. Test AddTorrentBytes
	var buf bytes.Buffer
	if err := mi.Write(&buf); err != nil {
		t.Fatalf("failed to serialize metainfo: %v", err)
	}

	tmpDir2, _ := os.MkdirTemp("", "idmm_torrent_bytes_*")
	defer os.RemoveAll(tmpDir2)
	cfg2 := DefaultClientConfig(tmpDir2)
	cfg2.DisableDHT = true
	cfg2.DisableTrackers = true
	client2, err := NewClient(cfg2)
	if err != nil {
		t.Fatalf("failed to create client2: %v", err)
	}
	defer client2.Close()

	dl2, err := client2.AddTorrentBytes(ctx, buf.Bytes())
	if err != nil {
		t.Fatalf("AddTorrentBytes failed: %v", err)
	}
	if dl2.InfoHash() != ih.HexString() {
		t.Errorf("infohash mismatch in AddTorrentBytes")
	}

	// 3. Test AddInput with .torrent file path
	dl3, err := client2.AddInput(ctx, torrentPath)
	if err != nil {
		t.Fatalf("AddInput with torrent path failed: %v", err)
	}
	if dl3.InfoHash() != ih.HexString() {
		t.Errorf("infohash mismatch in AddInput")
	}
}

func TestPeerToPeerDownload_TorrentFile(t *testing.T) {
	seederDir, err := os.MkdirTemp("", "idmm_seeder_*")
	if err != nil {
		t.Fatalf("failed to create seeder dir: %v", err)
	}
	defer os.RemoveAll(seederDir)

	downloaderDir, err := os.MkdirTemp("", "idmm_downloader_*")
	if err != nil {
		t.Fatalf("failed to create downloader dir: %v", err)
	}
	defer os.RemoveAll(downloaderDir)

	// Create test file with known SHA256 checksum
	content := []byte("Hello, this is a full End-to-End P2P Torrent transfer test executed by IDMM engine!")
	expectedHash := sha256.Sum256(content)

	torrentPath, _, _ := createSampleTorrent(t, seederDir, "p2p_test.txt", content)

	// 1. Setup Seeder Client
	seederCfg := DefaultClientConfig(seederDir)
	seederCfg.DisableDHT = true
	seederCfg.DisableTrackers = true
	seederCfg.Seed = true
	seederClient, err := NewClient(seederCfg)
	if err != nil {
		t.Fatalf("failed to create seeder client: %v", err)
	}
	defer seederClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	seederDl, err := seederClient.AddTorrentFile(ctx, torrentPath)
	if err != nil {
		t.Fatalf("seeder AddTorrentFile failed: %v", err)
	}
	seederDl.VerifyData()
	seederDl.Start()

	// 2. Setup Downloader Client
	downloaderCfg := DefaultClientConfig(downloaderDir)
	downloaderCfg.DisableDHT = true
	downloaderCfg.DisableTrackers = true
	downloaderClient, err := NewClient(downloaderCfg)
	if err != nil {
		t.Fatalf("failed to create downloader client: %v", err)
	}
	defer downloaderClient.Close()

	downloaderDl, err := downloaderClient.AddTorrentFile(ctx, torrentPath)
	if err != nil {
		t.Fatalf("downloader AddTorrentFile failed: %v", err)
	}

	// Connect downloader directly to seeder client
	downloaderDl.AddClientPeer(seederClient)
	downloaderDl.Start()

	// Wait for download completion (or timeout)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	completed := false
	for !completed {
		select {
		case <-timeout:
			t.Fatalf("timeout waiting for torrent download to complete; bytes completed: %d/%d, complete=%v",
				downloaderDl.BytesCompleted(), downloaderDl.TotalLength(), downloaderDl.IsComplete())
		case <-ticker.C:
			if downloaderDl.IsComplete() {
				completed = true
			}
		}
	}

	// 3. Verify downloaded file on disk
	downloadedFile := filepath.Join(downloaderDir, "p2p_test.txt")
	gotContent, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	gotHash := sha256.Sum256(gotContent)
	if gotHash != expectedHash {
		t.Fatalf("checksum mismatch: expected %x, got %x", expectedHash, gotHash)
	}
	fmt.Println("✅ .torrent file P2P download verified successfully!")
}

func TestPeerToPeerDownload_MagnetLink(t *testing.T) {
	seederDir, err := os.MkdirTemp("", "idmm_magnet_seeder_*")
	if err != nil {
		t.Fatalf("failed to create seeder dir: %v", err)
	}
	defer os.RemoveAll(seederDir)

	downloaderDir, err := os.MkdirTemp("", "idmm_magnet_downloader_*")
	if err != nil {
		t.Fatalf("failed to create downloader dir: %v", err)
	}
	defer os.RemoveAll(downloaderDir)

	content := []byte("Magnet metadata resolution and chunk transfer test via IDMM BitTorrent Client!")
	expectedHash := sha256.Sum256(content)

	torrentPath, ih, mi := createSampleTorrent(t, seederDir, "magnet_payload.txt", content)

	// Create Magnet URI from MetaInfo
	var info metainfo.Info
	if err := bencode.Unmarshal(mi.InfoBytes, &info); err != nil {
		t.Fatalf("failed to unmarshal info: %v", err)
	}
	mag := mi.Magnet(&ih, &info)
	magnetURI := mag.String()

	// 1. Setup Seeder Client
	seederCfg := DefaultClientConfig(seederDir)
	seederCfg.DisableDHT = true
	seederCfg.DisableTrackers = true
	seederCfg.Seed = true
	seederClient, err := NewClient(seederCfg)
	if err != nil {
		t.Fatalf("failed to create seeder client: %v", err)
	}
	defer seederClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	seederDl, err := seederClient.AddTorrentFile(ctx, torrentPath)
	if err != nil {
		t.Fatalf("seeder AddTorrentFile failed: %v", err)
	}
	seederDl.VerifyData()
	seederDl.Start()

	// 2. Setup Downloader Client with Magnet URI
	downloaderCfg := DefaultClientConfig(downloaderDir)
	downloaderCfg.DisableDHT = true
	downloaderCfg.DisableTrackers = true
	downloaderClient, err := NewClient(downloaderCfg)
	if err != nil {
		t.Fatalf("failed to create downloader client: %v", err)
	}
	defer downloaderClient.Close()

	downloaderDl, err := downloaderClient.AddMagnet(ctx, magnetURI)
	if err != nil {
		t.Fatalf("downloader AddMagnet failed: %v", err)
	}

	// Connect downloader to seeder to fetch metadata & pieces
	downloaderDl.AddClientPeer(seederClient)

	// Wait for metadata resolution via peer
	metaCtx, metaCancel := context.WithTimeout(ctx, 8*time.Second)
	defer metaCancel()

	if err := downloaderDl.WaitForMetadata(metaCtx); err != nil {
		t.Fatalf("WaitForMetadata failed: %v", err)
	}

	if downloaderDl.Name() != "magnet_payload.txt" {
		t.Errorf("expected torrent name 'magnet_payload.txt', got '%s'", downloaderDl.Name())
	}

	downloaderDl.Start()

	// Wait for piece download completion
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	completed := false
	for !completed {
		select {
		case <-timeout:
			t.Fatalf("timeout waiting for magnet download to complete; bytes: %d/%d, complete=%v",
				downloaderDl.BytesCompleted(), downloaderDl.TotalLength(), downloaderDl.IsComplete())
		case <-ticker.C:
			if downloaderDl.IsComplete() {
				completed = true
			}
		}
	}

	// 3. Verify downloaded file on disk
	downloadedFile := filepath.Join(downloaderDir, "magnet_payload.txt")
	gotContent, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("failed to read downloaded magnet file: %v", err)
	}

	gotHash := sha256.Sum256(gotContent)
	if gotHash != expectedHash {
		t.Fatalf("checksum mismatch: expected %x, got %x", expectedHash, gotHash)
	}
	fmt.Println("✅ Magnet link P2P end-to-end download verified successfully!")
}
