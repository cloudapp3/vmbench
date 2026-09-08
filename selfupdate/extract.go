package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// extractBinary streams the platform binary out of the release archive into
// dir as a synced temp file and returns its path. Windows releases are zip
// archives containing vmbench.exe; every other platform is tar.gz containing
// vmbench at the archive root.
func extractBinary(archivePath, dir string, mode os.FileMode, goos string) (string, error) {
	if goos == "windows" {
		return extractZipEntry(archivePath, dir, mode)
	}
	return extractTarEntry(archivePath, dir, mode)
}

func extractTarEntry(archivePath, dir string, mode os.FileMode) (string, error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	gz, err := gzip.NewReader(archive)
	if err != nil {
		return "", fmt.Errorf("read release archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read release archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Clean(header.Name) != binaryName {
			continue
		}
		return writeExtracted(reader, dir, mode)
	}
	return "", fmt.Errorf("release archive has no %s entry", binaryName)
}

func extractZipEntry(archivePath, dir string, mode os.FileMode) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("read release archive: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || filepath.Clean(file.Name) != binaryName+".exe" {
			continue
		}
		entry, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("read archive entry %s: %w", file.Name, err)
		}
		path, err := writeExtracted(entry, dir, mode)
		_ = entry.Close()
		return path, err
	}
	return "", fmt.Errorf("release archive has no %s.exe entry", binaryName)
}

func writeExtracted(reader io.Reader, dir string, mode os.FileMode) (returnPath string, returnErr error) {
	tmp, err := os.CreateTemp(dir, ".vmbench-*.tmp")
	if err != nil {
		return "", err
	}
	returnPath = tmp.Name()
	defer func() {
		_ = tmp.Close()
		if returnErr != nil {
			_ = os.Remove(returnPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, reader); err != nil {
		return "", fmt.Errorf("extract binary: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return returnPath, nil
}

// replaceBinary atomically moves the staged binary onto dest. Windows cannot
// rename over a running executable, so the current one is moved aside first
// and removed best-effort afterwards.
func replaceBinary(staged, dest, goos string) error {
	if goos != "windows" {
		if err := os.Rename(staged, dest); err != nil {
			return wrapReplaceError(fmt.Errorf("replace %s: %w", dest, err))
		}
		syncDir(filepath.Dir(dest))
		return nil
	}
	aside := dest + ".old"
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, aside); err != nil {
			return wrapReplaceError(fmt.Errorf("move aside %s: %w", dest, err))
		}
	}
	if err := os.Rename(staged, dest); err != nil {
		_ = os.Rename(aside, dest)
		return wrapReplaceError(fmt.Errorf("replace %s: %w", dest, err))
	}
	_ = os.Remove(aside)
	syncDir(filepath.Dir(dest))
	return nil
}

// wrapReplaceError marks permission failures so callers can suggest package
// manager alternatives.
func wrapReplaceError(err error) error {
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w: %v", ErrTargetNotWritable, err)
	}
	return err
}

func syncDir(dir string) {
	if handle, err := os.Open(dir); err == nil {
		_ = handle.Sync()
		_ = handle.Close()
	}
}
