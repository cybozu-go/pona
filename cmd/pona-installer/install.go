package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func installPona(ponaPath, cniBinDir string) error {
	f, err := os.Open(ponaPath)
	if err != nil {
		return fmt.Errorf("failed to read pona %w", err)
	}
	if err := os.MkdirAll(cniBinDir, 0755); err != nil {
		return fmt.Errorf("failed to MkdirAll: %w", err)
	}

	g, err := os.CreateTemp(cniBinDir, ".tmp")
	if err != nil {
		return fmt.Errorf("failed to CreateTemp: %w", err)
	}
	defer func() {
		g.Close()
		os.Remove(g.Name())
	}()

	if _, err := io.Copy(g, f); err != nil {
		return fmt.Errorf("failed to io.Copy: %w", err)
	}

	if err := g.Chmod(0755); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	if err := g.Sync(); err != nil {
		return fmt.Errorf("failed to Sync: %w", err)
	}

	if err := os.Rename(g.Name(), filepath.Join(cniBinDir, "pona")); err != nil {
		return fmt.Errorf("failed to rename: %w", err)
	}
	return nil
}
