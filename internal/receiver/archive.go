package receiver

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fwlog/internal/importer"
	"fwlog/internal/model"
)

var activeLogFilePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.log$`)

type ArchiveReadyKey struct {
	SourceID string
	Date     string
}

type ArchiveResult struct {
	SourceID    string    `json:"source_id"`
	Date        time.Time `json:"date"`
	Path        string    `json:"path"`
	Deleted     bool      `json:"deleted"`
	Error       string    `json:"error"`
	CompletedAt time.Time `json:"completed_at"`
}

type Archiver struct{}

func NewArchiver() *Archiver {
	return &Archiver{}
}

func (a *Archiver) Run(sources []model.LogSource, ready map[ArchiveReadyKey]bool, now time.Time) []ArchiveResult {
	results := make([]ArchiveResult, 0)
	for _, source := range sources {
		if !strings.EqualFold(strings.TrimSpace(source.SourceType), "rsyslog") || strings.TrimSpace(source.SpoolDir) == "" {
			continue
		}
		results = append(results, a.archiveClosedDays(source, now)...)
		results = append(results, a.removeExpiredArchives(source, ready, now)...)
	}
	return results
}

func (a *Archiver) archiveClosedDays(source model.LogSource, now time.Time) []ArchiveResult {
	spoolDir := filepath.Clean(strings.TrimSpace(source.SpoolDir))
	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []ArchiveResult{archiveError(source.SourceID, spoolDir, time.Time{}, now, err)}
	}

	today := startOfDay(now)
	results := make([]ArchiveResult, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := activeLogFilePattern.FindStringSubmatch(entry.Name())
		if len(matches) != 2 {
			continue
		}
		logDate, err := time.ParseInLocation("2006-01-02", matches[1], now.Location())
		if err != nil || !logDate.Before(today) {
			continue
		}

		sourcePath := filepath.Join(spoolDir, entry.Name())
		targetDir := archiveDirectory(source)
		targetPath := filepath.Join(targetDir, fmt.Sprintf(
			"%s_%s.log-%s.gz",
			source.SourceID,
			logDate.Format("2006-01-02"),
			now.Format("20060102"),
		))
		result := ArchiveResult{
			SourceID:    source.SourceID,
			Date:        logDate,
			Path:        targetPath,
			CompletedAt: now,
		}
		if err := archiveFile(sourcePath, targetPath); err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func archiveFile(sourcePath, targetPath string) error {
	if _, err := os.Stat(targetPath); err == nil {
		if err := validateGzip(targetPath); err != nil {
			return fmt.Errorf("已有归档不可读: %w", err)
		}
		if err := os.Remove(sourcePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除重复源文件: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查归档目标: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("创建归档目录: %w", err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("打开源日志: %w", err)
	}
	defer source.Close()

	temporary, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建归档临时文件: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	writer := gzip.NewWriter(temporary)
	if _, err := io.Copy(writer, source); err != nil {
		_ = writer.Close()
		return fmt.Errorf("压缩日志: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("完成 gzip 写入: %w", err)
	}
	if err := source.Close(); err != nil {
		return fmt.Errorf("关闭源日志: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("同步归档临时文件: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭归档临时文件: %w", err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("发布归档文件: %w", err)
	}
	removeTemporary = false
	if err := os.Remove(sourcePath); err != nil {
		return fmt.Errorf("删除已归档源日志: %w", err)
	}
	return nil
}

func validateGzip(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		_ = reader.Close()
		return err
	}
	return reader.Close()
}

func (a *Archiver) removeExpiredArchives(source model.LogSource, ready map[ArchiveReadyKey]bool, now time.Time) []ArchiveResult {
	if source.ArchiveRetentionDays <= 0 {
		return nil
	}
	archiveDir := archiveDirectory(source)
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []ArchiveResult{archiveError(source.SourceID, archiveDir, time.Time{}, now, err)}
	}

	cutoff := startOfDay(now).AddDate(0, 0, -source.ArchiveRetentionDays)
	prefix := source.SourceID + "_"
	results := make([]ArchiveResult, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !importer.IsArchivedLogFile(entry.Name()) {
			continue
		}
		logDate, ok := importer.ExtractLogDate(entry.Name())
		if !ok || !logDate.Before(cutoff) {
			continue
		}
		key := ArchiveReadyKey{SourceID: source.SourceID, Date: logDate.Format("2006-01-02")}
		if !ready[key] {
			continue
		}
		path := filepath.Join(archiveDir, entry.Name())
		result := ArchiveResult{
			SourceID:    source.SourceID,
			Date:        logDate,
			Path:        path,
			Deleted:     true,
			CompletedAt: now,
		}
		if err := os.Remove(path); err != nil {
			result.Deleted = false
			result.Error = fmt.Sprintf("删除过期归档: %v", err)
		}
		results = append(results, result)
	}
	return results
}

func archiveDirectory(source model.LogSource) string {
	if directory := strings.TrimSpace(source.ArchiveDir); directory != "" {
		return filepath.Clean(directory)
	}
	return filepath.Clean(strings.TrimSpace(source.SpoolDir))
}

func startOfDay(value time.Time) time.Time {
	local := value.In(value.Location())
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

func archiveError(sourceID, path string, date, completedAt time.Time, err error) ArchiveResult {
	return ArchiveResult{
		SourceID:    sourceID,
		Date:        date,
		Path:        path,
		Error:       err.Error(),
		CompletedAt: completedAt,
	}
}
