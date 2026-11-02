package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Browser profiles contain cookies and local storage. Keep them outside the
// task workspace, which the file manager can browse and download.
func prepareBrowserPrivateDir(keyDir, dataDir string) (string, error) {
	root := os.Getenv("ARTEX_BROWSER_AUTH_DIR")
	if root == "" {
		root = filepath.Join(keyDir, "browser-auth")
		if filepath.Clean(keyDir) == filepath.Clean(dataDir) {
			root = filepath.Join(filepath.Dir(keyDir), "."+filepath.Base(keyDir)+"-browser-auth")
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	workspace, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	if root == workspace || insideDirectory(workspace, root) {
		return "", errors.New("browser profiles must be outside the browsable task workspace")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("browser profile path is not a private directory")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve task workspace: %w", err)
	}
	if realRoot == realWorkspace || insideDirectory(realWorkspace, realRoot) {
		return "", errors.New("browser profiles resolve inside the browsable task workspace")
	}
	if err := os.Chmod(root, 0700); err != nil {
		return "", err
	}
	return root, nil
}

func insideDirectory(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
