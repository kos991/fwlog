package app

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

var (
	archivedLogFilePattern   = regexp.MustCompile(`^.+\.log-(\d{8})(?:\.gz)?$`)
	eventDatedLogFilePattern = regexp.MustCompile(`(?:^|[_-])(\d{4}-\d{2}-\d{2})\.log-\d{8}(?:\.gz)?$`)
)

type LogFileSnapshot struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
	LogDate time.Time
}

func IsArchivedLogFile(name string) bool {
	return archivedLogFilePattern.MatchString(name)
}

func ExtractLogDate(name string) (time.Time, bool) {
	if matches := eventDatedLogFilePattern.FindStringSubmatch(name); len(matches) == 2 {
		logDate, err := time.ParseInLocation("2006-01-02", matches[1], time.Local)
		if err == nil {
			return logDate, true
		}
	}

	matches := archivedLogFilePattern.FindStringSubmatch(name)
	if len(matches) != 2 {
		return time.Time{}, false
	}

	logDate, err := time.ParseInLocation("20060102", matches[1], time.Local)
	if err != nil {
		return time.Time{}, false
	}

	return logDate, true
}

func ScanArchivedLogFiles(root string, now time.Time) ([]LogFileSnapshot, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	files := make([]LogFileSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !IsArchivedLogFile(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if now.Sub(info.ModTime()) < 5*time.Minute {
			continue
		}

		logDate, ok := ExtractLogDate(entry.Name())
		if !ok {
			continue
		}

		files = append(files, LogFileSnapshot{
			Path:    filepath.Join(root, entry.Name()),
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			LogDate: logDate,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].LogDate.Equal(files[j].LogDate) {
			return files[i].Name < files[j].Name
		}
		return files[i].LogDate.Before(files[j].LogDate)
	})

	return files, nil
}
