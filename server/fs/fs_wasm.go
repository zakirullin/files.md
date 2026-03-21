//go:build wasm

package fs

import (
	"os"
)

var Ctime = func(fi os.FileInfo) int64 {
	if fi == nil {
		return 0
	}

	return fi.ModTime().UnixNano()
}

var Mtime = func(fi os.FileInfo) int64 {
	if fi == nil {
		return 0
	}

	return fi.ModTime().UnixNano()
}
