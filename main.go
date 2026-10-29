// pause copies the image payload from /seed<target> into each target directory,
// then waits for a signal.
//
// The targets are anonymous volumes that code-server reads via volumes_from.
// Docker fills such a volume from the image only when it creates the volume, and
// a plain `docker compose up -d` after a tag change keeps the old volume. Thus the
// image must refresh the volume itself.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
)

const seedRoot = "/seed"
const stampName = ".seed-stamp"

func main() {
	mounts := mountPoints()
	for _, target := range os.Args[1:] {
		if err := seed(seedRoot+target, target, mounts); err != nil {
			log.Fatalf("seed %s: %v", target, err)
		}
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c)
	<-c
}

// seed makes dst a copy of src. It does nothing when the stamp in dst shows that dst already has this payload.
func seed(src, dst string, mounts map[string]bool) error {
	want, err := fingerprint(src)
	if err != nil {
		return err
	}
	stamp := filepath.Join(dst, stampName)
	if have, err := os.ReadFile(stamp); err == nil && string(have) == want {
		log.Printf("%s is up to date", dst)
		return nil
	}
	log.Printf("refreshing %s", dst)
	if err := clear(dst, mounts); err != nil {
		return err
	}
	if err := copyTree(src, dst, mounts); err != nil {
		return err
	}
	// Write the stamp last: if the copy stops before it completes, the next start copies again.
	return os.WriteFile(stamp, []byte(want), 0o644)
}

// fingerprint hashes the path, mode, size and mtime of each entry. A new build changes at least one of them.
func fingerprint(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		fmt.Fprintf(h, "%s %v %d %d\n", rel, info.Mode(), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// clear removes the contents of dir. It keeps mount points (for example the gcloud-config volume) and the directories that contain them.
func clear(dir string, mounts map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		switch {
		case mounts[p]:
		case hasMountBelow(p, mounts):
			if err := clear(p, mounts); err != nil {
				return err
			}
		default:
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyTree(src, dst string, mounts map[string]bool) error {
	type dirMode struct {
		path string
		mode fs.FileMode
	}
	var dirs []dirMode
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		if p != src && mounts[out] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			dirs = append(dirs, dirMode{out, info.Mode().Perm()})
			return os.MkdirAll(out, 0o755)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, out)
		default:
			return copyFile(p, out, info.Mode().Perm())
		}
	})
	if err != nil {
		return err
	}
	// Set the directory modes last, so that a read-only directory does not block the copy of its contents.
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i].path, dirs[i].mode); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, perm) // the umask can remove bits from the mode given to OpenFile
}

// mountPoints reads the mount points from /proc/self/mountinfo.
// shortcut: it does not decode octal escapes (\040 = space). Decode them if a mount path can contain a space.
func mountPoints() map[string]bool {
	m := map[string]bool{}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) > 4 {
			m[f[4]] = true
		}
	}
	return m
}

func hasMountBelow(dir string, mounts map[string]bool) bool {
	for m := range mounts {
		if strings.HasPrefix(m, dir+"/") {
			return true
		}
	}
	return false
}
