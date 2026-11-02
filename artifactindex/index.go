package artifactindex

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxFiles      = 100
	MaxVisit      = 5000
	MaxFileBytes  = 20 << 20
	MaxTotalBytes = 50 << 20
)

type Artifact struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	ValueRank int    `json:"value_rank"`
}

// Collect indexes files already present in this task's workspace. It never
// follows a link outside that workspace and never sends bytes over the network.
func Collect(taskDir, target, pattern string, budget int64) ([]Artifact, error) {
	info, err := os.Lstat(taskDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("task workspace is unavailable")
	}
	rootPath, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return collect(root, rootPath, target, pattern, budget)
}

// CollectTask anchors the workspace under a trusted application data root.
// A task-controlled symlink cannot redirect the scan to another directory.
func CollectTask(workDir string, taskID int64, target, pattern string, budget int64) ([]Artifact, error) {
	if taskID <= 0 {
		return nil, errors.New("invalid task ID")
	}
	base, err := os.OpenRoot(workDir)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	rel := filepath.Join("tasks", fmt.Sprint(taskID))
	root, err := base.OpenRoot(rel)
	if err != nil {
		return nil, errors.New("task workspace is unavailable")
	}
	defer root.Close()
	return collect(root, filepath.Join(workDir, rel), target, pattern, budget)
}

func collect(root *os.Root, rootPath, target, pattern string, budget int64) ([]Artifact, error) {
	if budget <= 0 || budget > MaxTotalBytes {
		budget = MaxTotalBytes
	}
	if pattern != "" {
		if _, err := filepath.Match(pattern, "example"); err != nil {
			return nil, fmt.Errorf("invalid file pattern: %w", err)
		}
	}
	if target == "" {
		target = "."
	}
	if filepath.IsAbs(target) {
		var err error
		target, err = filepath.Rel(rootPath, target)
		if err != nil {
			return nil, err
		}
	}
	target = filepath.Clean(target)
	if target == ".." || strings.HasPrefix(target, ".."+string(filepath.Separator)) {
		return nil, errors.New("target path is outside the task workspace")
	}
	// Resolve the starting path before walking; WalkDir itself does not follow
	// symlinks, while Root.Open confines the eventual file read even if a path
	// changes during the walk.
	start := filepath.Join(rootPath, target)
	realStart, err := filepath.EvalSymlinks(start)
	if err != nil {
		return nil, err
	}
	if realStart != rootPath && !strings.HasPrefix(realStart, rootPath+string(filepath.Separator)) {
		return nil, errors.New("target path resolves outside the task workspace")
	}
	var out []Artifact
	var total int64
	visited := 0
	err = filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		visited++
		if visited > MaxVisit {
			return errors.New("too many files in task workspace")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if pattern != "" {
			match, _ := filepath.Match(pattern, filepath.Base(path))
			if !match {
				return nil
			}
		}
		if len(out) >= MaxFiles {
			return errors.New("too many matching evidence files")
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		file, err := root.Open(rel)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > MaxFileBytes || total+info.Size() > budget {
			file.Close()
			return errors.New("evidence size limit exceeded")
		}
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(file, MaxFileBytes+1))
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if n != info.Size() || n > MaxFileBytes || total+n > budget {
			return errors.New("evidence file changed or exceeded size limit")
		}
		total += n
		kind, rank := classify(filepath.Base(path))
		out = append(out, Artifact{Path: filepath.ToSlash(rel), Type: kind, SizeBytes: n, SHA256: hex.EncodeToString(hash.Sum(nil)), ValueRank: rank})
		return nil
	})
	return out, err
}

func classify(name string) (string, int) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".cred"), strings.HasSuffix(lower, ".credentials"), lower == "shadow", lower == "sam", lower == "ntds.dit":
		return "credential", 100
	case strings.HasSuffix(lower, ".token"), strings.HasSuffix(lower, ".jwt"), strings.HasSuffix(lower, ".key"):
		return "token", 90
	case strings.HasSuffix(lower, ".env"), strings.HasSuffix(lower, ".config"), strings.HasSuffix(lower, ".cfg"):
		return "config", 70
	case strings.HasSuffix(lower, ".dmp"), strings.HasSuffix(lower, ".dump"):
		return "dump", 60
	case strings.HasSuffix(lower, ".go"), strings.HasSuffix(lower, ".py"), strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".php"):
		return "source", 40
	case strings.HasSuffix(lower, ".png"), strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "screenshot", 20
	default:
		return "file", 10
	}
}
