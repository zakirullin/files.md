//go:build windows && 386
// +build windows,386

package w32

func GetWindowLong(hwnd uintptr, index int) uintptr {
	ret, _, _ := User32GetWindowLongW.Call(hwnd, uintptr(index))
	return ret
}

func SetWindowLong(hwnd uintptr, index int, newLong uintptr) uintptr {
	ret, _, _ := User32SetWindowLongW.Call(hwnd, uintptr(index), newLong)
	return ret
}
