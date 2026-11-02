package artifactindex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectConfinesAndHashesTaskFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "downloads"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "downloads", "sample.env"), []byte("canary=value"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.env")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "downloads", "link.env")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	rows, err := Collect(root, "downloads", "*.env", MaxTotalBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Path != "downloads/sample.env" || rows[0].Type != "config" || rows[0].SHA256 == "" {
		t.Fatalf("unexpected task evidence: %+v", rows)
	}
	if _, err := Collect(root, outside, "", MaxTotalBytes); err == nil {
		t.Fatal("outside file was accepted")
	}
	if _, err := Collect(root, "..", "", MaxTotalBytes); err == nil {
		t.Fatal("parent directory was accepted")
	}
}

func TestCollectTaskRejectsTaskDirectorySymlink(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "tasks"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.env"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "tasks", "7")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := CollectTask(base, 7, ".", "", MaxTotalBytes); err == nil {
		t.Fatal("task directory symlink escaped workspace")
	}
}
