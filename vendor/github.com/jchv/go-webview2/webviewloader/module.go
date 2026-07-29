package webviewloader

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/jchv/go-winloader"
	"golang.org/x/sys/windows"
)

var (
	nativeModule                                       = windows.NewLazyDLL("WebView2Loader")
	nativeCreate                                       = nativeModule.NewProc("CreateCoreWebView2EnvironmentWithOptions")
	nativeCompareBrowserVersions                       = nativeModule.NewProc("CompareBrowserVersions")
	nativeGetAvailableCoreWebView2BrowserVersionString = nativeModule.NewProc("GetAvailableCoreWebView2BrowserVersionString")

	memOnce                                         sync.Once
	memModule                                       winloader.Module
	memCreate                                       winloader.Proc
	memCompareBrowserVersions                       winloader.Proc
	memGetAvailableCoreWebView2BrowserVersionString winloader.Proc
	memErr                                          error
)

// CompareBrowserVersions will compare the 2 given versions and return:
//
//	-1 = v1 < v2
//	 0 = v1 == v2
//	 1 = v1 > v2
func CompareBrowserVersions(v1 string, v2 string) (int, error) {

	_v1, err := windows.UTF16PtrFromString(v1)
	if err != nil {
		return 0, err
	}
	_v2, err := windows.UTF16PtrFromString(v2)
	if err != nil {
		return 0, err
	}

	nativeErr := nativeModule.Load()
	if nativeErr == nil {
		nativeErr = nativeCompareBrowserVersions.Find()
	}
	var result int
	if nativeErr != nil {
		err = loadFromMemory(nativeErr)
		if err != nil {
			return 0, fmt.Errorf("Unable to load WebView2Loader.dll from disk: %v -- or from memory: %w", nativeErr, memErr)
		}
		_, _, err = memCompareBrowserVersions.Call(
			uint64(uintptr(unsafe.Pointer(_v1))),
			uint64(uintptr(unsafe.Pointer(_v2))),
			uint64(uintptr(unsafe.Pointer(&result))))
	} else {
		_, _, err = nativeCompareBrowserVersions.Call(
			uintptr(unsafe.Pointer(_v1)),
			uintptr(unsafe.Pointer(_v2)),
			uintptr(unsafe.Pointer(&result)))
	}
	if err != windows.ERROR_SUCCESS {
		return result, err
	}
	return result, nil
}

// GetInstalledVersion returns the installed version of the webview2 runtime.
// If there is no version installed, a blank string is returned.
func GetInstalledVersion() (string, error) {
	// GetAvailableCoreWebView2BrowserVersionString is documented as:
	//	public STDAPI GetAvailableCoreWebView2BrowserVersionString(PCWSTR browserExecutableFolder, LPWSTR * versionInfo)
	// where winnt.h defines STDAPI as:
	//	EXTERN_C HRESULT STDAPICALLTYPE
	// the first part (EXTERN_C) can be ignored since it's only relevent to C++,
	// HRESULT is return type which means it returns an integer that will be 0 (S_OK) on success,
	// and finally STDAPICALLTYPE tells us the function uses the stdcall calling convention (what Go assumes for syscalls).

	nativeErr := nativeModule.Load()
	if nativeErr == nil {
		nativeErr = nativeGetAvailableCoreWebView2BrowserVersionString.Find()
	}
	var hr uintptr
	var result *uint16
	if nativeErr != nil {
		if err := loadFromMemory(nativeErr); err != nil {
			return "", fmt.Errorf("Unable to load WebView2Loader.dll from disk: %v -- or from memory: %w", nativeErr, memErr)
		}
		hr64, _, _ := memGetAvailableCoreWebView2BrowserVersionString.Call(
			uint64(uintptr(unsafe.Pointer(nil))),
			uint64(uintptr(unsafe.Pointer(&result))))
		hr = uintptr(hr64) // The return size of the HRESULT will be whatver native size is (i.e uintptr) and not 64-bits on 32-bit systems.  In both cases it should be interpreted as 32-bits (a LONG).
	} else {
		hr, _, _ = nativeGetAvailableCoreWebView2BrowserVersionString.Call(
			uintptr(unsafe.Pointer(nil)),
			uintptr(unsafe.Pointer(&result)))
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(result)) // Safe even if result is nil
	if hr != uintptr(windows.S_OK) {
		if hr&0xFFFF == uintptr(windows.ERROR_FILE_NOT_FOUND) {
			// The lower 16-bits (the error code itself) of the HRESULT is ERROR_FILE_NOT_FOUND which means the system isn't installed.
			return "", nil // Return a blank string but no error since we successfully detected no install.
		}
		return "", fmt.Errorf("GetAvailableCoreWebView2BrowserVersionString returned HRESULT 0x%X", hr)
	}
	version := windows.UTF16PtrToString(result) // Safe even if result is nil
	return version, nil
}

// CreateCoreWebView2EnvironmentWithOptions tries to load WebviewLoader2 and
// call the CreateCoreWebView2EnvironmentWithOptions routine.
func CreateCoreWebView2EnvironmentWithOptions(browserExecutableFolder, userDataFolder *uint16, environmentOptions uintptr, environmentCompletedHandle uintptr) (uintptr, error) {
	nativeErr := nativeModule.Load()
	if nativeErr == nil {
		nativeErr = nativeCreate.Find()
	}
	if nativeErr != nil {
		err := loadFromMemory(nativeErr)
		if err != nil {
			return 0, err
		}
		res, _, _ := memCreate.Call(
			uint64(uintptr(unsafe.Pointer(browserExecutableFolder))),
			uint64(uintptr(unsafe.Pointer(userDataFolder))),
			uint64(environmentOptions),
			uint64(environmentCompletedHandle),
		)
		return uintptr(res), nil
	}
	res, _, _ := nativeCreate.Call(
		uintptr(unsafe.Pointer(browserExecutableFolder)),
		uintptr(unsafe.Pointer(userDataFolder)),
		environmentOptions,
		environmentCompletedHandle,
	)
	return res, nil
}

func loadFromMemory(nativeErr error) error {
	var err error
	// DLL is not available natively. Try loading embedded copy.
	memOnce.Do(func() {
		memModule, memErr = winloader.LoadFromMemory(WebView2Loader)
		if memErr != nil {
			err = fmt.Errorf("Unable to load WebView2Loader.dll from disk: %v -- or from memory: %w", nativeErr, memErr)
			return
		}
		memCreate = memModule.Proc("CreateCoreWebView2EnvironmentWithOptions")
		memCompareBrowserVersions = memModule.Proc("CompareBrowserVersions")
		memGetAvailableCoreWebView2BrowserVersionString = memModule.Proc("GetAvailableCoreWebView2BrowserVersionString")
	})
	return err
}
