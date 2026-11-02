package c2bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxBinaryBytes = 50 << 20

type StagedBinary struct {
	Arch      string `json:"arch"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// Stage verifies an explicitly configured local binary. It neither copies the
// file into a browsable task directory nor transfers it to a target.
func Stage(arch string) (StagedBinary, error) {
	var key string
	switch arch {
	case "x64":
		key = "ARTEX_C2_BIN_X64"
	case "x86":
		key = "ARTEX_C2_BIN_X86"
	default:
		return StagedBinary{}, errors.New("architecture must be x64 or x86")
	}
	path := os.Getenv(key)
	if path == "" || !filepath.IsAbs(path) {
		return StagedBinary{}, errors.New("binary path must be explicitly configured as an absolute path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return StagedBinary{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > MaxBinaryBytes {
		return StagedBinary{}, errors.New("binary must be a nonempty regular file no larger than 50 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return StagedBinary{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != info.Size() || !os.SameFile(info, stat) {
		return StagedBinary{}, errors.New("binary changed during verification")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, MaxBinaryBytes+1))
	if err != nil || n != info.Size() {
		return StagedBinary{}, errors.New("binary changed during verification")
	}
	return StagedBinary{Arch: arch, Name: strings.TrimSpace(filepath.Base(path)), SizeBytes: n, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}
