//go:build linux

package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDMIInfoAtCollectsIdentity(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"product_name": "Alibaba Cloud ECS\n",
		"sys_vendor":   "Alibaba Cloud",
		"board_vendor": "Alibaba Cloud",
		"board_name":   "PC-Compatible",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := readDMIInfoAt(dir)
	want := DMIInfo{
		ProductName: "Alibaba Cloud ECS",
		SysVendor:   "Alibaba Cloud",
		BoardVendor: "Alibaba Cloud",
		BoardName:   "PC-Compatible",
	}
	if got != want {
		t.Fatalf("readDMIInfoAt = %+v, want %+v", got, want)
	}
}

func TestReadDMIInfoAtFiltersJunkAndMissing(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"product_name": "To Be Filled By O.E.M.\n",
		"sys_vendor":   "  \n",
		"board_name":   "Unknown",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := readDMIInfoAt(dir); got != (DMIInfo{}) {
		t.Fatalf("readDMIInfoAt should drop junk/blank DMI values, got %+v", got)
	}
	// Entirely absent DMI tree (containers) yields the zero value, no error.
	if got := readDMIInfoAt(filepath.Join(dir, "missing")); got != (DMIInfo{}) {
		t.Fatalf("readDMIInfoAt on missing dir = %+v, want zero", got)
	}
}
