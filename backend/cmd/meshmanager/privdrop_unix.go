//go:build unix

package main

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// dropPrivileges makes the process run as PUID:PGID when the container starts as root,
// the way linuxserver.io images do. It first gives that user ownership of DATA_DIR, so
// a bind mount that Docker created as root is writable. Without root (e.g. compose
// `user:`) it does nothing.
func dropPrivileges(dataDir string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	uid, err := idEnv("PUID", 1000)
	if err != nil {
		return err
	}
	gid, err := idEnv("PGID", 1000)
	if err != nil {
		return err
	}
	if uid == 0 {
		slog.Warn("PUID=0: running as root")
		return nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}
	if err := chownTree(dataDir, uid, gid); err != nil {
		return fmt.Errorf("set owner of %s to %d:%d: %w", dataDir, uid, gid, err)
	}

	if err := syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid %d: %w", gid, err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid %d: %w", uid, err)
	}
	slog.Info("running as unprivileged user", "uid", uid, "gid", gid)
	return nil
}

// chownTree hands root and everything below it to uid:gid. It skips the walk when root
// already has the right owner, so restarts stay fast with large thumbnail folders.
func chownTree(root string, uid, gid int) error {
	st, err := os.Stat(root)
	if err != nil {
		return err
	}
	if s, ok := st.Sys().(*syscall.Stat_t); ok && int(s.Uid) == uid && int(s.Gid) == gid {
		return nil
	}
	slog.Info("fixing ownership of data folder", "path", root, "uid", uid, "gid", gid)
	return filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(p, uid, gid)
	})
}

func idEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number, got %q", key, v)
	}
	return n, nil
}
