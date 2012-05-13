package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
)

// {{{ main.

func main() {
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprint(os.Stderr, "Usage:\n  icasefs MOUNTPOINT ORIGDIR\n")
		os.Exit(1)
	}

	origDir, err := filepath.Abs(flag.Arg(1))
	if err != nil {
		log.Fatalf("Error resolving ORIGDIR: %v", err)
	}

	fs := NewFS(origDir)
	nfs := fuse.NewPathNodeFs(fs, nil)
	state, _, err := fuse.MountNodeFileSystem(flag.Arg(0), nfs, nil)
	if err != nil {
		log.Fatalf("Mount fail: %v", err)
	}

	state.Loop()
}

// }}} main.

// {{{ type FS.

type FS struct {
	fuse.LoopbackFileSystem
}

func NewFS(root string) *FS {
	return &FS{
		LoopbackFileSystem: fuse.LoopbackFileSystem{Root: root},
	}
}

// {{{ Methods implementing fuse.FileSystem.

func (fs *FS) GetAttr(name string, context *fuse.Context) (attr *fuse.Attr, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		attr, code = fs.LoopbackFileSystem.GetAttr(nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

// {{{ Extended attributes.

func (fs *FS) GetXAttr(name string, attribute string, context *fuse.Context) (data []byte, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		data, code = fs.LoopbackFileSystem.GetXAttr(nameAttempt, attribute, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) ListXAttr(name string, context *fuse.Context) (attributes []string, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		attributes, code = fs.LoopbackFileSystem.ListXAttr(nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) RemoveXAttr(name string, attr string, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.RemoveXAttr(nameAttempt, attr, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) SetXAttr(name string, attr string, data []byte, flags int, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.SetXAttr(nameAttempt, attr, data, flags, context)
		return code == fuse.ENOENT
	})
	return
}

// }}} Extended attributes.

// {{{ File handling.

func (fs *FS) Open(name string, flags uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		file, code = fs.LoopbackFileSystem.Open(nameAttempt, flags, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		file, code = fs.LoopbackFileSystem.Create(nameAttempt, flags, mode, context)
		// TODO consider case of creating a new file in a directory whose path's
		// case mismatches.
		return code == fuse.ENOENT
	})
	return
}

// }}} File handling.

// {{{ Directory handling.

func (fs *FS) OpenDir(name string, context *fuse.Context) (c chan fuse.DirEntry, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		c, code = fs.LoopbackFileSystem.OpenDir(nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

// }}} Directory handling.

// }}} Methods implementing fuse.FileSystem.

// {{{ Utility methods.

// CaseMatchingRetry attempts the operation for the given path with
// case-insentive retry. If the operation returns true, then it attempts to
// find a new case-insensitive path, and will attempt the operation once again
// with a match - iff it finds one. Note that even if this second attempt
// returns true, it will not be called a third time.
func (fs *FS) CaseMatchingRetry(name string, op func(string) bool) {
	if !op(name) {
		return
	}

	matchedName, code := fs.MatchAndLogIcasePath(name)
	if code.Ok() {
		op(matchedName)
	}
}

func (fs *FS) MatchAndLogIcasePath(name string) (matchedName string, code fuse.Status) {
	// TODO Consider cache of recent successful matches (failures more risky so not worth it?).
	matchedName, found, err := fs.FindMatchingIcasePaths(name)
	if err != nil {
		log.Printf("error searching for %q: %v", name, err)
		return "", fuse.ToStatus(err)
	} else if !found {
		// TODO Remove this case, it's not interesting and might get spammy.
		log.Printf("no match found for %q", name)
		return "", fuse.ENOENT
	}
	// TODO notify of summarized matches found in a useful way
	log.Printf("match found for %q: %q", name, matchedName)
	return matchedName, fuse.OK
}

func (fs *FS) FindMatchingIcasePaths(name string) (matchedName string, found bool, err error) {
	dirPath, fileName := filepath.Split(name)
	realDirPath := fs.LoopbackFileSystem.GetPath(dirPath)
	dir, err := os.Open(realDirPath)
	if err != nil {
		// TODO deal with case where the directory's name is mismatched case (recurse on realDirPath)
		return "", false, err
	}
	defer dir.Close()

	maxEntries := 100
	var dirEntries []string
	lowerFileName := strings.ToLower(fileName)
	for err = nil; err == nil; dirEntries, err = dir.Readdirnames(maxEntries) {
		for _, entry := range dirEntries {
			if lowerFileName == strings.ToLower(entry) {
				// Found a match.
				return filepath.Join(dirPath, entry), true, nil
				// TODO deal with case of potentially multiple matches. matchIcasePath
				// should return ([]string,error) instead of (string,bool,error).
			}
		}
	}

	if err == io.EOF {
		// No real error, no match found.
		return "", false, nil
	}
	// Broke on reading directory entries.
	return "", false, err
}

// }}} Utility methods.

// }}} type FS.
