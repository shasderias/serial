//go:build ignore

package serial

//go:generate cmd /C "go tool cgo -godefs $GOFILE > ztypes_windows.go"

// #include <windows.h>
// #include <winbase.h>
import "C"

const (
	maxDWORD = C.MAXDWORD
)

// https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-dcb

const (
	cbr110    = C.CBR_110
	cbr300    = C.CBR_300
	cbr600    = C.CBR_600
	cbr1200   = C.CBR_1200
	cbr2400   = C.CBR_2400
	cbr4800   = C.CBR_4800
	cbr9600   = C.CBR_9600
	cbr14400  = C.CBR_14400
	cbr19200  = C.CBR_19200
	cbr38400  = C.CBR_38400
	cbr57600  = C.CBR_57600
	cbr115200 = C.CBR_115200
	cbr128000 = C.CBR_128000
	cbr256000 = C.CBR_256000
)

const (
	dtrControlDisable   = C.DTR_CONTROL_DISABLE
	dtrControlEnable    = C.DTR_CONTROL_ENABLE
	dtrControlHandshake = C.DTR_CONTROL_HANDSHAKE
)

const (
	rtsControlDisable   = C.RTS_CONTROL_DISABLE
	rtsControlEnable    = C.RTS_CONTROL_ENABLE
	rtsControlHandshake = C.RTS_CONTROL_HANDSHAKE
	rtsControlToggle    = C.RTS_CONTROL_TOGGLE
)

const (
	oneStopBit  = C.ONESTOPBIT
	twoStopBits = C.TWOSTOPBITS
)

const (
	evenParity = C.EVENPARITY
	oddParity  = C.ODDPARITY
	noParity   = C.NOPARITY
)

type dcb C.DCB

func toDWORD(val int) C.DWORD {
	return C.DWORD(val)
}

func toBYTE(val int) C.BYTE {
	return C.BYTE(val)
}
