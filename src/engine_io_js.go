//go:build js

package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// In-memory virtual filesystem populated from a fetched asset zip.
// Engine reads route here via engineOpen/engineReadFile/engineStat.
// Bundle layout matches the native Ikemen-GO release: chars/, stages/,
// data/, font/, sound/, etc., at the zip root.
//
// Paths are normalized by stripping leading "./" so "data/system.def"
// and "./data/system.def" resolve identically.

var (
	vfsMu      sync.RWMutex
	vfsEntries = map[string][]byte{}
)

// LoadAssetZip parses a zip archive and populates the VFS. Call once
// at startup from JS via window.ikemen.loadAssetBundle(bytes).
func LoadAssetZip(buf []byte) (int, error) {
	r, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return 0, err
	}
	vfsMu.Lock()
	defer vfsMu.Unlock()
	loaded := 0
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return loaded, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return loaded, err
		}
		vfsEntries[normalizeVFSPath(f.Name)] = b
		loaded++
	}
	return loaded, nil
}

func normalizeVFSPath(p string) string {
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	// Some engine paths use backslashes from MUGEN-style configs.
	p = strings.ReplaceAll(p, "\\", "/")
	return path.Clean(p)
}

func vfsLookup(p string) ([]byte, bool) {
	n := normalizeVFSPath(p)
	vfsMu.RLock()
	b, ok := vfsEntries[n]
	vfsMu.RUnlock()
	return b, ok
}

// vfsFile wraps a byte slice and implements io.ReadSeekCloser.
type vfsFile struct {
	data   []byte
	offset int64
}

func (f *vfsFile) Read(p []byte) (int, error) {
	if f.offset >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *vfsFile) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = f.offset + offset
	case io.SeekEnd:
		abs = int64(len(f.data)) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("negative offset")
	}
	f.offset = abs
	return abs, nil
}

func (f *vfsFile) Close() error {
	return nil
}

func engineOpen(p string) (io.ReadSeekCloser, error) {
	b, ok := vfsLookup(p)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	// Return a copy so callers can't mutate VFS entry via seek/read.
	buf := make([]byte, len(b))
	copy(buf, b)
	return &vfsFile{data: buf}, nil
}

func engineReadFile(p string) ([]byte, error) {
	b, ok := vfsLookup(p)
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: p, Err: fs.ErrNotExist}
	}
	// Return a copy so callers can't mutate the VFS entry.
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

type vfsFileInfo struct {
	name string
	size int64
}

func (i *vfsFileInfo) Name() string       { return i.name }
func (i *vfsFileInfo) Size() int64        { return i.size }
func (i *vfsFileInfo) Mode() os.FileMode  { return 0444 }
func (i *vfsFileInfo) ModTime() time.Time { return time.Time{} }
func (i *vfsFileInfo) IsDir() bool        { return false }
func (i *vfsFileInfo) Sys() interface{}   { return nil }

func engineStat(p string) (os.FileInfo, error) {
	b, ok := vfsLookup(p)
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
	}
	return &vfsFileInfo{name: path.Base(p), size: int64(len(b))}, nil
}

// VFSEntryCount and VFSHas exposed for diagnostics.
func VFSEntryCount() int {
	vfsMu.RLock()
	defer vfsMu.RUnlock()
	return len(vfsEntries)
}

func VFSHas(p string) bool {
	_, ok := vfsLookup(p)
	return ok
}

// Avoid unused-import warning until something calls errors.X.
var _ = errors.New
