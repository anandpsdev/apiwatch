package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anandpsdev/apiwatch/config"
	apierrors "github.com/anandpsdev/apiwatch/errors"
	"github.com/anandpsdev/apiwatch/src/filter"
	"github.com/anandpsdev/apiwatch/src/lock"
	"github.com/anandpsdev/apiwatch/src/types"
)

type FileStore struct {
	mu             sync.RWMutex
	cfg            config.Config
	index          *Index
	activeFile     *os.File
	activeFilePath string
	activeFileSize int64
	activeDay      string
	closed         bool
}

func NewFileStore(cfg config.Config) (*FileStore, error) {
	cfg = cfg.Normalize()

	if err := os.MkdirAll(cfg.File.Directory, 0755); err != nil {
		return nil, fmt.Errorf("apiwatch: failed to create log directory: %w", err)
	}

	fs := &FileStore{
		cfg:   cfg,
		index: NewIndex(),
	}

	if err := fs.rebuildIndex(); err != nil {
		return nil, fmt.Errorf("apiwatch: index rebuilding failed: %w", err)
	}

	if err := fs.initActiveFile(); err != nil {
		return nil, fmt.Errorf("apiwatch: active file initialization failed: %w", err)
	}

	fs.enforceRetention()

	return fs, nil
}

func (fs *FileStore) rebuildIndex() error {
	files, err := fs.findLogFiles()
	if err != nil {
		return err
	}

	sort.Strings(files)

	for _, file := range files {
		if err := fs.indexFile(file); err != nil {
			if fs.cfg.OnError != nil {
				fs.cfg.OnError(fmt.Errorf("apiwatch: error indexing file %s: %w", file, err))
			}
		}
	}

	return nil
}

func (fs *FileStore) indexFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 64*1024)
	var offset int64 = 0
	var lineBuf bytes.Buffer

	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			start := 0
			for i, b := range chunk {
				if b == '\n' {
					lineBuf.Write(chunk[start : i+1])
					lineBytes := lineBuf.Bytes()
					lineLen := int64(len(lineBytes))

					trimmed := bytes.TrimSpace(lineBytes)
					if len(trimmed) > 0 {
						var summary struct {
							ID string `json:"id"`
						}
						if err := json.Unmarshal(trimmed, &summary); err == nil && summary.ID != "" {
							fs.index.Set(summary.ID, RecordLocation{
								File:   path,
								Offset: offset,
								Length: lineLen,
							})
						} else if fs.cfg.OnError != nil {
							fs.cfg.OnError(fmt.Errorf("apiwatch: corrupted line in %s at offset %d: %w", path, offset, err))
						}
					}

					offset += lineLen
					lineBuf.Reset()
					start = i + 1
				}
			}
			if start < n {
				lineBuf.Write(chunk[start:])
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				if lineBuf.Len() > 0 {
					lineBytes := lineBuf.Bytes()
					lineLen := int64(len(lineBytes))
					trimmed := bytes.TrimSpace(lineBytes)
					if len(trimmed) > 0 {
						var summary struct {
							ID string `json:"id"`
						}
						if err := json.Unmarshal(trimmed, &summary); err == nil && summary.ID != "" {
							fs.index.Set(summary.ID, RecordLocation{
								File:   path,
								Offset: offset,
								Length: lineLen,
							})
						}
					}
				}
				break
			}
			return readErr
		}
	}

	return nil
}

func (fs *FileStore) findLogFiles() ([]string, error) {
	entries, err := os.ReadDir(fs.cfg.File.Directory)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "apiwatch-") && strings.HasSuffix(name, ".jsonl") {
			files = append(files, filepath.Join(fs.cfg.File.Directory, name))
		}
	}
	return files, nil
}

func (fs *FileStore) initActiveFile() error {
	today := time.Now().Format("2006-01-02")
	targetPath := fs.nextAvailableFilePath(today)

	f, err := lock.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, os.FileMode(fs.cfg.File.FilePermissions), fs.cfg.File.LockExternalAccess)
	if err != nil {
		return err
	}

	fi, err := f.Stat()
	if err != nil {
		_ = lock.UnlockAndClose(f)
		return err
	}

	fs.activeFile = f
	fs.activeFilePath = targetPath
	fs.activeFileSize = fi.Size()
	fs.activeDay = today

	return nil
}

func (fs *FileStore) nextAvailableFilePath(day string) string {
	basePrefix := filepath.Join(fs.cfg.File.Directory, fmt.Sprintf("apiwatch-%s", day))
	defaultPath := basePrefix + ".jsonl"

	fi, err := os.Stat(defaultPath)
	if os.IsNotExist(err) {
		return defaultPath
	}
	if err == nil && fi.Size() < fs.cfg.Retention.MaxFileSize {
		return defaultPath
	}

	seq := 1
	for {
		candidate := fmt.Sprintf("%s.%d.jsonl", basePrefix, seq)
		cfi, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			return candidate
		}
		if err == nil && cfi.Size() < fs.cfg.Retention.MaxFileSize {
			return candidate
		}
		seq++
	}
}

func (fs *FileStore) Append(ctx context.Context, entry types.Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("apiwatch: failed to marshal entry: %w", err)
	}
	data = append(data, '\n')
	entryLen := int64(len(data))

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return apierrors.ErrStoreClosed
	}

	today := time.Now().Format("2006-01-02")
	if today != fs.activeDay || (fs.activeFileSize+entryLen > fs.cfg.Retention.MaxFileSize) {
		if err := fs.rotateLocked(today); err != nil {
			return fmt.Errorf("apiwatch: rotation failed: %w", err)
		}
	}

	offset := fs.activeFileSize
	n, err := fs.activeFile.Write(data)
	if err != nil {
		return fmt.Errorf("apiwatch: write failed: %w", err)
	}
	fs.activeFileSize += int64(n)

	fs.index.Set(entry.ID, RecordLocation{
		File:   fs.activeFilePath,
		Offset: offset,
		Length: int64(n),
	})

	return nil
}

func (fs *FileStore) rotateLocked(today string) error {
	if fs.activeFile != nil {
		_ = lock.UnlockAndClose(fs.activeFile)
		fs.activeFile = nil
	}

	targetPath := fs.nextAvailableFilePath(today)
	f, err := lock.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, os.FileMode(fs.cfg.File.FilePermissions), fs.cfg.File.LockExternalAccess)
	if err != nil {
		return err
	}

	fi, err := f.Stat()
	if err != nil {
		_ = lock.UnlockAndClose(f)
		return err
	}

	fs.activeFile = f
	fs.activeFilePath = targetPath
	fs.activeFileSize = fi.Size()
	fs.activeDay = today

	fs.enforceRetentionLocked()

	return nil
}

func (fs *FileStore) enforceRetention() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.enforceRetentionLocked()
}

func (fs *FileStore) enforceRetentionLocked() {
	files, err := fs.findLogFiles()
	if err != nil {
		return
	}

	type fileInfo struct {
		path    string
		size    int64
		modTime time.Time
	}

	var nonActiveFiles []fileInfo
	var totalSize int64 = fs.activeFileSize
	totalFiles := 1

	for _, p := range files {
		if p == fs.activeFilePath {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		nonActiveFiles = append(nonActiveFiles, fileInfo{
			path:    p,
			size:    fi.Size(),
			modTime: fi.ModTime(),
		})
		totalSize += fi.Size()
		totalFiles++
	}
	sort.Slice(nonActiveFiles, func(i, j int) bool {
		return nonActiveFiles[i].modTime.Before(nonActiveFiles[j].modTime)
	})

	now := time.Now()
	var remaining []fileInfo
	for _, fi := range nonActiveFiles {
		if fs.cfg.Retention.MaxAge > 0 && now.Sub(fi.modTime) > fs.cfg.Retention.MaxAge {
			_ = os.Remove(fi.path)
			fs.index.RemoveFile(fi.path)
			totalSize -= fi.size
			totalFiles--
		} else {
			remaining = append(remaining, fi)
		}
	}

	for len(remaining) > 0 && totalFiles > fs.cfg.Retention.MaxFiles {
		oldest := remaining[0]
		_ = os.Remove(oldest.path)
		fs.index.RemoveFile(oldest.path)
		totalSize -= oldest.size
		totalFiles--
		remaining = remaining[1:]
	}

	for len(remaining) > 0 && totalSize > fs.cfg.Retention.MaxTotalSize {
		oldest := remaining[0]
		_ = os.Remove(oldest.path)
		fs.index.RemoveFile(oldest.path)
		totalSize -= oldest.size
		totalFiles--
		remaining = remaining[1:]
	}
}

func (fs *FileStore) Get(ctx context.Context, id string) (types.Entry, error) {
	fs.mu.RLock()
	loc, ok := fs.index.Get(id)
	activePath := fs.activeFilePath
	activeFile := fs.activeFile
	closed := fs.closed
	fs.mu.RUnlock()

	if closed {
		return types.Entry{}, apierrors.ErrStoreClosed
	}
	if !ok {
		return types.Entry{}, apierrors.ErrNotFound
	}

	return fs.readEntry(loc, activePath, activeFile)
}

func (fs *FileStore) readEntry(loc RecordLocation, activePath string, activeFile *os.File) (types.Entry, error) {
	buf := make([]byte, loc.Length)
	var err error

	if loc.File == activePath && activeFile != nil {
		_, err = activeFile.ReadAt(buf, loc.Offset)
	} else {
		var f *os.File
		f, err = os.Open(loc.File)
		if err != nil {
			return types.Entry{}, err
		}
		defer f.Close()
		_, err = f.ReadAt(buf, loc.Offset)
	}

	if err != nil && err != io.EOF {
		return types.Entry{}, fmt.Errorf("apiwatch: failed to read record: %w", err)
	}

	var entry types.Entry
	trimmed := bytes.TrimSpace(buf)
	if err := json.Unmarshal(trimmed, &entry); err != nil {
		return types.Entry{}, fmt.Errorf("apiwatch: failed to decode entry: %w", err)
	}

	return entry, nil
}

func (fs *FileStore) List(ctx context.Context, f types.Filter) ([]types.Entry, error) {
	items, _, err := fs.ListWithTotal(ctx, f)
	return items, err
}

func (fs *FileStore) ListWithTotal(ctx context.Context, f types.Filter) ([]types.Entry, int, error) {
	f = f.Normalize()

	fs.mu.RLock()
	ids := fs.index.IDs()
	activePath := fs.activeFilePath
	activeFile := fs.activeFile
	closed := fs.closed
	fs.mu.RUnlock()

	if closed {
		return nil, 0, apierrors.ErrStoreClosed
	}

	var matchedItems []types.Entry
	matchedCount := 0

	for _, id := range ids {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		default:
		}

		fs.mu.RLock()
		loc, ok := fs.index.Get(id)
		fs.mu.RUnlock()
		if !ok {
			continue
		}

		entry, err := fs.readEntry(loc, activePath, activeFile)
		if err != nil {
			if fs.cfg.OnError != nil {
				fs.cfg.OnError(err)
			}
			continue
		}

		if filter.Matches(entry, f) {
			if matchedCount >= f.Offset && len(matchedItems) < f.Limit {
				matchedItems = append(matchedItems, entry)
			}
			matchedCount++
		}
	}

	return matchedItems, matchedCount, nil
}

func (fs *FileStore) Clear(ctx context.Context) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return apierrors.ErrStoreClosed
	}

	if fs.activeFile != nil {
		_ = lock.UnlockAndClose(fs.activeFile)
		fs.activeFile = nil
	}

	fs.index.Clear()

	files, err := fs.findLogFiles()
	if err == nil {
		for _, file := range files {
			_ = os.Remove(file)
		}
	}

	return fs.initActiveFile()
}

func (fs *FileStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return nil
	}
	fs.closed = true

	if fs.activeFile != nil {
		err := lock.UnlockAndClose(fs.activeFile)
		fs.activeFile = nil
		return err
	}
	return nil
}
