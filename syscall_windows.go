//go:build ignore

package serial

//go:generate mkwinsyscall -output zsyscall_windows.go $GOFILE

//sys getCommState(handle windows.Handle, dcb *dcb) (err error) = GetCommState
//sys setCommState(handle windows.Handle, dcb *dcb) (err error) = SetCommState
