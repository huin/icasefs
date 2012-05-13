package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/raw"
)

// {{{ flags.

var (
	logFilename = flag.String("log_filename", "", "Log output. Defaults to stderr.")
)

// }}} flags.

// {{{ main and sundries.

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage:\n  icasefs [options] MOUNTPOINT ORIGDIR\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	if logFile, err := configureLogging(); err != nil {
		log.Fatalf("Error configuring logging: %v", err)
	} else if logFile != nil {
		defer logFile.Close()
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

func configureLogging() (logFile *os.File, err error) {
	if *logFilename != "" {
		logFile, err = os.OpenFile(*logFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		log.SetOutput(logFile)
	}
	return
}

// }}} main and sundries.

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

// {{{ Simple operations.

func (fs *FS) GetAttr(name string, context *fuse.Context) (attr *fuse.Attr, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		attr, code = fs.LoopbackFileSystem.GetAttr(nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Chmod(nameAttempt, mode, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Chown(nameAttempt, uid, gid, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Utimens(name string, AtimeNs int64, MtimeNs int64, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Utimens(nameAttempt, AtimeNs, MtimeNs, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Truncate(name string, size uint64, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Truncate(nameAttempt, size, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Access(nameAttempt, mode, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Rmdir(nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Unlink(nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

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

func (fs *FS) Open(name string, flags uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		file, code = fs.LoopbackFileSystem.Open(nameAttempt, flags, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) OpenDir(name string, context *fuse.Context) (c chan fuse.DirEntry, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		c, code = fs.LoopbackFileSystem.OpenDir(nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Readlink(name string, context *fuse.Context) (value string, code fuse.Status) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		value, code = fs.LoopbackFileSystem.Readlink(nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) StatFs(name string) (out *fuse.StatfsOut) {
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		// Copied from LoopbackFileSystem.StatFs, as the return code doesn't give
		// an indication for error reason.
		s := syscall.Statfs_t{}
		err := syscall.Statfs(fs.GetPath(name), &s)
		out = nil
		if err == nil {
			out = &fuse.StatfsOut{
				raw.Kstatfs{
					Blocks:  s.Blocks,
					Bsize:   uint32(s.Bsize),
					Bfree:   s.Bfree,
					Bavail:  s.Bavail,
					Files:   s.Files,
					Ffree:   s.Ffree,
					Frsize:  uint32(s.Frsize),
					NameLen: uint32(s.Namelen),
				},
			}
			return false
		}
		out = nil

		return os.IsNotExist(err)
	})
	return
}

// }}} Simple operations.

// {{{ Creation operations.

/* TODO func (fs *FS) Link(oldName string, newName string, context *fuse.Context) (code fuse.Status)

func (fs *FS) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status

func (fs *FS) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) fuse.Status*/

func (fs *FS) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	log.Printf("Create(%q, %d, %d, %#v)", name, flags, mode, context)
	fs.CaseMatchingRetry(name, func(nameAttempt string) bool {
		file, code = fs.LoopbackFileSystem.Create(nameAttempt, flags, mode, context)
		// TODO consider case of creating a new file in a directory whose path's
		// case mismatches.
		return code == fuse.ENOENT
	})
	return
}

// TODO func (fs *FS) Symlink(value string, linkName string, context *fuse.Context) (code fuse.Status)

// }}} Creation operations.

// {{{ Complex operations.

// TODO func (fs *FS) Rename(oldName string, newName string, context *fuse.Context) (code fuse.Status)

// }}} Complex operations.

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
	// TODO Consider a cache of recent successful matches, but not for failures.

	matchedNames, err := fs.FindMatchingIcasePaths(name)
	if err != nil {
		log.Printf("error while searching for %q: %v", name, err)
		return "", fuse.ToStatus(err)
	} else if len(matchedNames) == 0 {
		return "", fuse.ENOENT
	}

	// TODO Notify of summarized matches found in a useful/parseable way.

	if len(matchedNames) > 1 {
		log.Printf("%d matches found for %q, using first", len(matchedNames), name)
	}
	log.Printf("match found for %q: %q", name, matchedNames[0])
	return matchedNames[0], fuse.OK
}

func (fs *FS) FindMatchingIcasePaths(name string) (matchedNames []string, err error) {
	if name == "" {
		return nil, nil
	}

	dirPath, fileName := filepath.Split(name)
	if dirPath != "" && dirPath[len(dirPath)-1] == filepath.Separator {
		dirPath = dirPath[:len(dirPath)-1]
	}

	lowerFileName := strings.ToLower(fileName)

	dir, err := os.Open(fs.LoopbackFileSystem.GetPath(dirPath))
	if err == nil {
		// The directory could be opened okay.
		return dirScan(dirPath, dir, lowerFileName, matchedNames)
	} else if !os.IsNotExist(err) {
		// General error opening directory.
		return nil, err
	}

	// Directory (or a parent) is a potentially mismatched case.
	searchDirPaths, err := fs.FindMatchingIcasePaths(dirPath)
	if err != nil {
		return nil, err
	}

	for _, possibleDirPath := range searchDirPaths {
		dir, err := os.Open(fs.LoopbackFileSystem.GetPath(possibleDirPath))
		if err != nil {
			return nil, err
		}

		matchedNames, err = dirScan(possibleDirPath, dir, lowerFileName, matchedNames)
		if err != nil {
			return nil, err
		}
	}

	return matchedNames, nil
}

// Helper function to scan a directory that might potentially contain a
// matching file. Closes dir on return.
func dirScan(dirPath string, dir *os.File, lowerFileName string, matchedNames []string) ([]string, error) {
	defer dir.Close()

	var dirEntries []string
	var err error

	maxEntries := 100
	for err = nil; err == nil; dirEntries, err = dir.Readdirnames(maxEntries) {
		for _, entry := range dirEntries {
			if lowerFileName == strings.ToLower(entry) {
				// Found a match.
				matchedNames = append(matchedNames, filepath.Join(dirPath, entry))
			}
		}
	}
	if err != io.EOF {
		// Broke on reading directory entries.
		return nil, err
	}
	return matchedNames, nil
}

// }}} Utility methods.

// }}} type FS.
