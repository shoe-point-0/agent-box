package spike

import (
	"fmt"
	"os"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
)

func writeSyncedFile(path string, data []byte, mode os.FileMode) error {
	return atomicfile.WriteFile(path, data, mode)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("sync directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close synced directory: %w", err)
	}
	return nil
}
