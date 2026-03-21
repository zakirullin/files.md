//go:build darwin

package fs

import (
	"os"
	"syscall"
)

var Ctime = func(fi os.FileInfo) int64 {
	stat := fi.Sys().(*syscall.Stat_t)

	return (stat.Ctimespec.Sec*1_000_000_000 + stat.Ctimespec.Nsec) / 1000 // Look for CONFIG_HZ in README.md
}

var Mtime = func(fi os.FileInfo) int64 {
	stat := fi.Sys().(*syscall.Stat_t)

	return (stat.Mtimespec.Sec*1_000_000_000 + stat.Mtimespec.Nsec) / 1000 // Look for CONFIG_HZ in README.md
}
