package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// readTaskFile confines structured evidence reads to the current task directory.
// Raw evidence and secrets stay out of tool arguments and transcript output.
func (t *ToolSet) readTaskFile(name string, maxBytes int64) ([]byte, error) {
	if t.workDir == "" || t.taskID <= 0 || !filepath.IsLocal(name) || name == "." || maxBytes <= 0 {
		return nil, errors.New("a local task evidence file is required")
	}
	base, err := os.OpenRoot(t.workDir)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	root, err := base.OpenRoot(filepath.Join("tasks", fmt.Sprint(t.taskID)))
	if err != nil {
		return nil, errors.New("task workspace is unavailable")
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, errors.New("evidence file is not regular or exceeds the size limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return nil, errors.New("evidence file exceeds the size limit or could not be read")
	}
	return data, nil
}
