//go:build !go1.18
// +build !go1.18

package edge

import "syscall"

//go:uintptrescapes
// Call calls a COM procedure.
func (p ComProc) Call(a ...uintptr) (r1, r2 uintptr, lastErr error) {
	// The magic uintptrescapes comment is needed to prevent moving uintptr(unsafe.Pointer(p)) so calls to .Call() also
	// satisfy the unsafe.Pointer rule "(4) Conversion of a Pointer to a uintptr when calling syscall.Syscall."
	// Otherwise it might be that pointers get moved, especially pointer onto the Go stack which might grow dynamically.
	// See https://pkg.go.dev/unsafe#Pointer and https://github.com/golang/go/issues/34474
	switch len(a) {
	case 0:
		return syscall.Syscall(uintptr(p), 0, 0, 0, 0)
	case 1:
		return syscall.Syscall(uintptr(p), 1, a[0], 0, 0)
	case 2:
		return syscall.Syscall(uintptr(p), 2, a[0], a[1], 0)
	case 3:
		return syscall.Syscall(uintptr(p), 3, a[0], a[1], a[2])
	case 4:
		return syscall.Syscall6(uintptr(p), 4, a[0], a[1], a[2], a[3], 0, 0)
	case 5:
		return syscall.Syscall6(uintptr(p), 5, a[0], a[1], a[2], a[3], a[4], 0)
	case 6:
		return syscall.Syscall6(uintptr(p), 6, a[0], a[1], a[2], a[3], a[4], a[5])
	case 7:
		return syscall.Syscall9(uintptr(p), 7, a[0], a[1], a[2], a[3], a[4], a[5], a[6], 0, 0)
	case 8:
		return syscall.Syscall9(uintptr(p), 8, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], 0)
	case 9:
		return syscall.Syscall9(uintptr(p), 9, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8])
	case 10:
		return syscall.Syscall12(uintptr(p), 10, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9], 0, 0)
	case 11:
		return syscall.Syscall12(uintptr(p), 11, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9], a[10], 0)
	case 12:
		return syscall.Syscall12(uintptr(p), 12, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9], a[10], a[11])
	case 13:
		return syscall.Syscall15(uintptr(p), 13, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9], a[10], a[11], a[12], 0, 0)
	case 14:
		return syscall.Syscall15(uintptr(p), 14, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9], a[10], a[11], a[12], a[13], 0)
	case 15:
		return syscall.Syscall15(uintptr(p), 15, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9], a[10], a[11], a[12], a[13], a[14])
	default:
		panic("too many arguments")
	}
}
