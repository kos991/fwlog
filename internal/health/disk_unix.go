//go:build !windows

package health

import (
	"syscall"
)

func collectDiskHealth(path string) DiskHealth {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil || stat.Blocks == 0 {
		return DiskHealth{Path: path}
	}
	total := uint64(stat.Blocks) * uint64(stat.Bsize)
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	return DiskHealth{Path: path, TotalBytes: total, AvailableBytes: available, AvailablePercent: diskAvailablePercent(total, available)}
}

func diskAvailablePercent(total, available uint64) float64 {
	if total == 0 {
		return 0
	}
	return round1(float64(available) / float64(total) * 100)
}
