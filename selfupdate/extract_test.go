package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func buildTarGz(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	for name, data := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
			t.Fatalf("write tar header %s: %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("write tar entry %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buffer.Bytes()
}

func TestExtractTarMatchesRootBinaryOnly(t *testing.T) {
	archive := buildTarGz(t, map[string][]byte{
		"LICENSE":        []byte("license"),
		"docs/README.md": []byte("readme"),
		"./vmbench":      marker,
	})
	archivePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	staged, err := extractBinary(archivePath, t.TempDir(), 0o755, "linux")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(staged) })
	data, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("read staged: %v", err)
	}
	if !bytes.Equal(data, marker) {
		t.Errorf("staged content = %q, want marker", data)
	}
}

func TestExtractTarWithoutBinary(t *testing.T) {
	archive := buildTarGz(t, map[string][]byte{"README.md": []byte("readme")})
	archivePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	_, err := extractBinary(archivePath, t.TempDir(), 0o755, "linux")
	if err == nil {
		t.Fatal("extractBinary without vmbench entry should fail")
	}
}

func TestExtractZipWithoutBinary(t *testing.T) {
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	entry, _ := zw.Create("README.md")
	_, _ = entry.Write([]byte("readme"))
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	archivePath := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(archivePath, buffer.Bytes(), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	_, err := extractBinary(archivePath, t.TempDir(), 0o755, "windows")
	if err == nil {
		t.Fatal("extractBinary without vmbench.exe entry should fail")
	}
}

func TestExtractSetsRequestedMode(t *testing.T) {
	archive := buildTarGz(t, map[string][]byte{"vmbench": marker})
	archivePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	staged, err := extractBinary(archivePath, t.TempDir(), 0o700, "linux")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(staged) })
	info, err := os.Stat(staged)
	if err != nil {
		t.Fatalf("stat staged: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("staged mode = %o, want 700", info.Mode().Perm())
	}
}

func TestReplaceBinaryMovesAsideOnWindows(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "vmbench.exe")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	staged := filepath.Join(dir, ".vmbench-1.tmp")
	if err := os.WriteFile(staged, marker, 0o755); err != nil {
		t.Fatalf("seed staged: %v", err)
	}
	if err := replaceBinary(staged, dest, "windows"); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(data, marker) {
		t.Errorf("dest content = %q, want marker", data)
	}
	if _, err := os.Stat(dest + ".old"); !os.IsNotExist(err) {
		t.Errorf("aside file should be removed (err=%v)", err)
	}
}

func TestReplaceBinaryAtomicOnUnix(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "vmbench")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	staged := filepath.Join(dir, ".vmbench-1.tmp")
	if err := os.WriteFile(staged, marker, 0o755); err != nil {
		t.Fatalf("seed staged: %v", err)
	}
	if err := replaceBinary(staged, dest, "linux"); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if !bytes.Equal(data, marker) {
		t.Errorf("dest content = %q, want marker", data)
	}
}
