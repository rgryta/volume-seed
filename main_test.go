package main

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestSeed(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	keep := filepath.Join(dst, "cfg", "keep")
	mounts := map[string]bool{keep: true}

	write(t, filepath.Join(src, "bin", "tool"), "v1")
	write(t, filepath.Join(src, "old"), "v1")
	if err := os.Symlink("/opt/tools/bin/tool", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(keep, "creds"), "secret") // the content of a mounted volume
	write(t, filepath.Join(dst, "stale"), "from an older image")

	// First start: copy the payload, remove the stale file, keep the mount.
	if err := seed(src, dst, mounts); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "bin", "tool")); string(b) != "v1" {
		t.Fatalf("tool = %q, want v1", b)
	}
	if info, _ := os.Stat(filepath.Join(dst, "bin", "tool")); info.Mode().Perm() != 0o755 {
		t.Fatalf("tool mode = %v, want 0755", info.Mode().Perm())
	}
	if link, _ := os.Readlink(filepath.Join(dst, "link")); link != "/opt/tools/bin/tool" {
		t.Fatalf("link = %q", link)
	}
	if exists(filepath.Join(dst, "stale")) {
		t.Fatal("stale file was not removed")
	}
	if !exists(filepath.Join(keep, "creds")) {
		t.Fatal("mounted volume content was removed")
	}

	// Restart with the same image: do not change dst.
	write(t, filepath.Join(dst, "marker"), "x")
	if err := seed(src, dst, mounts); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dst, "marker")) {
		t.Fatal("unchanged payload caused a new copy")
	}

	// New image: update the changed files, remove the deleted files, keep the mount.
	write(t, filepath.Join(src, "bin", "tool"), "v2")
	if err := os.Remove(filepath.Join(src, "old")); err != nil {
		t.Fatal(err)
	}
	if err := seed(src, dst, mounts); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "bin", "tool")); string(b) != "v2" {
		t.Fatalf("tool = %q, want v2", b)
	}
	if exists(filepath.Join(dst, "old")) || exists(filepath.Join(dst, "marker")) {
		t.Fatal("files from the previous payload are still there")
	}
	if !exists(filepath.Join(keep, "creds")) {
		t.Fatal("mounted volume content was removed on refresh")
	}
}
