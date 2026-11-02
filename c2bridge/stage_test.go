package c2bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStageRequiresExplicitRegularBinary(t *testing.T) {
	t.Setenv("ARTEX_C2_BIN_X64", "")
	if _, err := Stage("x64"); err == nil {
		t.Fatal("unconfigured binary was accepted")
	}
	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, []byte("canary fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARTEX_C2_BIN_X64", path)
	staged, err := Stage("x64")
	if err != nil || staged.SHA256 == "" || staged.Name != "fixture.bin" || staged.SizeBytes != 14 {
		t.Fatalf("stage fixture: %+v %v", staged, err)
	}
	link := filepath.Join(t.TempDir(), "link.bin")
	if err := os.Symlink(path, link); err == nil {
		t.Setenv("ARTEX_C2_BIN_X64", link)
		if _, err := Stage("x64"); err == nil {
			t.Fatal("symlinked binary was accepted")
		}
	}
}
