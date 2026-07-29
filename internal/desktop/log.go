package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	coreLogMaxSizeMB  = 10
	coreLogMaxBackups = 5
	coreLogMaxAgeDays = 14
)

func newCoreLogWriter(path string) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    coreLogMaxSizeMB,
		MaxBackups: coreLogMaxBackups,
		MaxAge:     coreLogMaxAgeDays,
		Compress:   true,
		LocalTime:  true,
	}
}

type LogCleanupResult struct {
	Files int
	Bytes int64
}

func (c *Controller) LogUsage() (int64, error) {
	if c == nil {
		return 0, nil
	}
	entries, err := os.ReadDir(filepath.Dir(c.logFile))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("读取日志目录：%w", err)
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !isCoreLogFile(c.logFile, entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, fmt.Errorf("读取日志文件信息：%w", err)
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	return total, nil
}

func (c *Controller) ClearOldLogs() (LogCleanupResult, error) {
	if c == nil {
		return LogCleanupResult{}, nil
	}
	directory := filepath.Dir(c.logFile)
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return LogCleanupResult{}, nil
	}
	if err != nil {
		return LogCleanupResult{}, fmt.Errorf("读取日志目录：%w", err)
	}
	active := filepath.Base(c.logFile)
	result := LogCleanupResult{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == active || !isCoreLogFile(c.logFile, entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return result, fmt.Errorf("读取历史日志信息：%w", err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		target := filepath.Join(directory, entry.Name())
		if filepath.Dir(target) != filepath.Clean(directory) {
			return result, errors.New("历史日志路径越过日志目录")
		}
		if err := os.Remove(target); err != nil {
			return result, fmt.Errorf("删除历史日志 %s：%w", entry.Name(), err)
		}
		result.Files++
		result.Bytes += info.Size()
	}
	return result, nil
}

func isCoreLogFile(activePath, name string) bool {
	active := filepath.Base(activePath)
	if name == active {
		return true
	}
	extension := filepath.Ext(active)
	prefix := strings.TrimSuffix(active, extension) + "-"
	return strings.HasPrefix(name, prefix) &&
		(strings.HasSuffix(name, extension) || strings.HasSuffix(name, extension+".gz"))
}

func humanBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KB", "MB", "GB"}
	size := float64(value)
	for _, unit := range units {
		size /= 1024
		if size < 1024 || unit == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
	}
	return fmt.Sprintf("%d B", value)
}
