//go:build windows && (amd64 || arm64)
// +build windows
// +build amd64 arm64

package w32

func GetWindowLong(hwnd uintptr, index int) uintptr {
	ret, _, _ := User32GetWindowLongPtrW.Call(hwnd, uintptr(index))
	return ret
}

func SetWindowLong(hwnd uintptr, index int, newLong uintptr) uintptr {
	ret, _, _ := User32SetWindowLongPtrW.Call(hwnd, uintptr(index), newLong)
	return ret
}
