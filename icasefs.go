package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/raw"
)

// {{{ flags.

var (
	logFilename    = flag.String("log_filename", "", "Log output. Defaults to stderr.")
	reportFilename = flag.String("report_filename", "", "Record case insensitive matches to this file.")
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

	fs := NewFS(origDir, *reportFilename)
	nfs := fuse.NewPathNodeFs(fs, nil)
	state, _, err := fuse.MountNodeFileSystem(flag.Arg(0), nfs, nil)
	if err != nil {
		log.Fatalf("Mount fail: %v", err)
	}

	state.Loop()

	err = fs.WriteReport()
	if err != nil {
		log.Printf("Failed to write matches: %v", err)
	}
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

// {{{ type Report.

type Report struct {
	mutex sync.Mutex

	outputFilename string

	// Map from a matching path to those that were attempted.
	matchedFiles map[string][]string

	// The set of attempted paths.
	pathsSet map[string]struct{}
}

func NewReport(outputFilename string) *Report {
	return &Report{
		outputFilename: outputFilename,
		matchedFiles:   make(map[string][]string),
		pathsSet:       make(map[string]struct{}),
	}
}

func (report *Report) MergeMatchedNames(path string, matchedNames []string) {
	if len(matchedNames) == 0 {
		return
	}

	report.mutex.Lock()
	defer report.mutex.Unlock()

	if _, alreadyExists := report.pathsSet[path]; !alreadyExists {
		report.pathsSet[path] = struct{}{}

		for _, matchedName := range matchedNames {
			report.matchedFiles[matchedName] = append(report.matchedFiles[matchedName], path)
		}
	}
}

func (report *Report) WriteReport() error {
	report.mutex.Lock()
	defer report.mutex.Unlock()

	reportFile, err := os.OpenFile(report.outputFilename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer reportFile.Close()

	encoder := json.NewEncoder(reportFile)
	encoder.Encode(report.matchedFiles)

	return nil
}

// }}} type Report.

// {{{ type FS.

type FS struct {
	fuse.LoopbackFileSystem
	report *Report
}

func NewFS(root, reportFilename string) *FS {
	fs := &FS{
		LoopbackFileSystem: fuse.LoopbackFileSystem{Root: root},
	}
	if reportFilename != "" {
		fs.report = NewReport(reportFilename)
	}
	return fs
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
//
// ENOENT in these cases are caused by a parent of the target not existing,
// rather than the target itself.

func (fs *FS) Mkdir(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	fs.ParentCaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Mkdir(nameAttempt, mode, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) (code fuse.Status) {
	fs.ParentCaseMatchingRetry(name, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Mknod(nameAttempt, mode, dev, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	fs.ParentCaseMatchingRetry(name, func(nameAttempt string) bool {
		file, code = fs.LoopbackFileSystem.Create(nameAttempt, flags, mode, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Symlink(value string, linkName string, context *fuse.Context) (code fuse.Status) {
	fs.ParentCaseMatchingRetry(linkName, func(nameAttempt string) bool {
		code = fs.LoopbackFileSystem.Symlink(value, nameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

// }}} Creation operations.

// {{{ Complex operations.

func (fs *FS) Link(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	fs.OldNewCaseMatchingRetry(oldName, newName, func(oldNameAttempt, newNameAttempt string) bool {
		code = fs.LoopbackFileSystem.Link(oldNameAttempt, newNameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

func (fs *FS) Rename(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	fs.OldNewCaseMatchingRetry(oldName, newName, func(oldNameAttempt, newNameAttempt string) bool {
		code = fs.LoopbackFileSystem.Rename(oldNameAttempt, newNameAttempt, context)
		return code == fuse.ENOENT
	})
	return
}

// }}} Complex operations.

// }}} Methods implementing fuse.FileSystem.

// {{{ Case matching methods.

// CaseMatchingRetry attempts the operation for the given path with
// case-insensitive retry. If the operation returns true, then it attempts to
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

// ParentCaseMatchingRetry is similar to CaseMatchingRetry, but attempts case
// matching on the parent directory, without trying to match on the base name.
func (fs *FS) ParentCaseMatchingRetry(name string, op func(string) bool) {
	if !op(name) {
		return
	}

	matchedName, code := fs.ParentMatchAndLogIcasePath(name)
	if code.Ok() {
		op(matchedName)
	}
}

// OldNewCaseMatchingRetry is similar to CaseMatchingRetry, but attempts case
// matching on oldName, and parent case matching on newName.
func (fs *FS) OldNewCaseMatchingRetry(oldName, newName string, op func(string, string) bool) {

	if !op(oldName, newName) {
		return
	}

	var code fuse.Status

	// We need oldName to exist.
	if _, err := os.Stat(oldName); err != nil {
		if os.IsNotExist(err) {
			oldName, code = fs.MatchAndLogIcasePath(oldName)
			if !code.Ok() {
				return
			}
		} else {
			return
		}
	}

	// We need the parent of newName to exist.
	if parentNewName, _ := pathSplit(newName); parentNewName != "" {
		if _, err := os.Stat(parentNewName); err != nil {
			if os.IsNotExist(err) {
				newName, code = fs.ParentMatchAndLogIcasePath(newName)
				if !code.Ok() {
					return
				}
			} else {
				return
			}
		}
	}

	op(oldName, newName)
}

func (fs *FS) MatchAndLogIcasePath(name string) (matchedName string, code fuse.Status) {
	// TODO Consider a cache of recent successful matches, but not for failures.
	// Remember that method invocations run in separate goroutines.

	matchedNames, err := fs.FindMatchingIcasePaths(name)
	if err != nil {
		log.Printf("error while searching for %q: %v", name, err)
		return "", fuse.ToStatus(err)
	} else if len(matchedNames) == 0 {
		return "", fuse.ENOENT
	}

	if len(matchedNames) > 1 {
		log.Printf("%d matches found for %q, using first", len(matchedNames), name)
	}
	log.Printf("match found for %q: %q", name, matchedNames[0])
	return matchedNames[0], fuse.OK
}

// ParentMatchAndLogIcasePath is the same as MatchAndLogIcasePath, but attempts
// case matching on the parent directory, without trying to match on the base
// name.
func (fs *FS) ParentMatchAndLogIcasePath(name string) (matchedName string, code fuse.Status) {
	dirPath, fileName := pathSplit(name)

	matchedDirPath, code := fs.MatchAndLogIcasePath(dirPath)
	if !code.Ok() {
		return "", code
	}

	return filepath.Join(matchedDirPath, fileName), code
}

func (fs *FS) FindMatchingIcasePaths(name string) (matchedNames []string, err error) {
	if name == "" {
		return nil, nil
	}

	dirPath, fileName := pathSplit(name)

	lowerFileName := strings.ToLower(fileName)

	dir, err := os.Open(fs.LoopbackFileSystem.GetPath(dirPath))
	if err == nil {
		// The directory could be opened okay.
		matchedNames, err = dirScan(dirPath, dir, lowerFileName, matchedNames)
		if err != nil {
			fs.MergeMatchedNames(name, matchedNames)
		}
		return
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
		fs.MergeMatchedNames(name, matchedNames)
	}

	return matchedNames, nil
}

func (fs *FS) MergeMatchedNames(path string, matchedNames []string) {
	if fs.report != nil {
		fs.report.MergeMatchedNames(path, matchedNames)
	}
}

func (fs *FS) WriteReport() error {
	if fs.report != nil {
		return fs.report.WriteReport()
	}
	return nil
}

// }}} Case matching methods.

// }}} type FS.

// {{{ Utility functions.

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

// pathSplit is similar to filepath.Split, but removes any trailing / from the
// directory component.
func pathSplit(name string) (dir, file string) {
	dir, file = filepath.Split(name)
	if dir != "" && dir[len(dir)-1] == filepath.Separator {
		dir = dir[:len(dir)-1]
	}
	return dir, file
}

// }}} Utility functions.
