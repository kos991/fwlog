//go:build windows

package health

func collectDiskHealth(path string) DiskHealth {
	return DiskHealth{Path: path}
}
