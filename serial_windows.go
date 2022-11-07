package serial

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const (
	maxDWORD = 0xffffffff
)

type dcb struct {
	// https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-dcb

	DCBLength uint32
	BaudRate  uint32

	// Flags is a bitfield
	// DWORD fBinary : 1;
	// DWORD fParity : 1;
	// DWORD fOutxCtsFlow : 1;
	// DWORD fOutxDsrFlow : 1;
	// DWORD fDtrControl : 2;
	// DWORD fDsrSensitivity : 1;
	// DWORD fTXContinueOnXoff : 1;
	// DWORD fOutX : 1;
	// DWORD fInX : 1;
	// DWORD fErrorChar : 1;
	// DWORD fNull : 1;
	// DWORD fRtsControl : 2;
	// DWORD fAbortOnError : 1;
	// DWORD fDummy2 : 17;
	Flags uint32

	WReserved  uint16
	XonLim     uint16
	XoffLim    uint16
	ByteSize   uint8
	Parity     uint8
	StopBits   uint8
	XonChar    int8
	XoffChar   int8
	ErrorChar  int8
	EOFChar    int8
	EvtChar    int8
	WReserved1 uint16
}

var baudRates = map[int]uint32{
	0: cbr19200, // default

	110:    cbr110,
	300:    cbr300,
	600:    cbr600,
	1200:   cbr1200,
	2400:   cbr2400,
	4800:   cbr4800,
	9600:   cbr9600,
	14400:  cbr14400,
	19200:  cbr19200,
	38400:  cbr38400,
	57600:  cbr57600,
	115200: cbr115200,
	128000: cbr128000,
	256000: cbr256000,
}

const (
	cbr110    = 0x6e
	cbr300    = 0x12c
	cbr600    = 0x258
	cbr1200   = 0x4b0
	cbr2400   = 0x960
	cbr4800   = 0x12c0
	cbr9600   = 0x2580
	cbr14400  = 0x3840
	cbr19200  = 0x4b00
	cbr38400  = 0x9600
	cbr57600  = 0xe100
	cbr115200 = 0x1c200
	cbr128000 = 0x1f400
	cbr256000 = 0x3e800
)

const (
	dcbfBinary           = 0b01 << 0
	dcbfParity           = 0b01 << 1
	dcbfOutxCTSFlow      = 0b01 << 2
	dcbfOutxDSRFlow      = 0b01 << 3
	dcbfDTRControl       = 0b11 << 4
	dcbfDSRSensitivity   = 0b01 << 6
	dcbfTXContinueOnXoff = 0b01 << 7
	dcbfOutX             = 0b01 << 8
	dcbfInX              = 0b01 << 9
	dcbfErrorChar        = 0b01 << 10
	dcbfNull             = 0b01 << 11
	dcbfRTSControl       = 0b11 << 12
	dcbfAbortOnError     = 0b01 << 14
)

const (
	dtrControlDisable   = 0x0
	dtrControlEnable    = 0x1
	dtrControlHandshake = 0x2
)

const (
	rtsControlDisable   = 0x0
	rtsControlEnable    = 0x1
	rtsControlHandshake = 0x2
	rtsControlToggle    = 0x3
)

const (
	oneStopBit  = 0x0
	twoStopBits = 0x2
)

const (
	evenParity = 0x2
	oddParity  = 0x1
	noParity   = 0x0
)

type port struct {
	mut    sync.RWMutex
	handle windows.Handle

	ro, wo        *windows.Overlapped
	readDeadline  time.Time
	writeDeadline time.Time
}

func nativeOpen(path string, conf *Config) (*port, error) {
	// required when using CreateFile to get a handle to a device
	// https://learn.microsoft.com/en-us/windows/win32/devio/communications-resource-handles
	const pathPrefix = `\\.\`

	path = pathPrefix + path

	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(path),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,                            //exclusive access
		nil,                          // default security attributes
		windows.OPEN_EXISTING,        // must be OPEN_EXISTING
		windows.FILE_FLAG_OVERLAPPED, // overlapped I/O
		0,                            // must be NULL for comm devices
	)
	if err != nil {
		return nil, err
	}

	var d dcb

	if err := getCommState(handle, &d); err != nil {
		return nil, err
	}

	dcbInit(&d)
	if err := dcbSetBaudRate(&d, conf.BaudRate); err != nil {
		return nil, err
	}
	if err := dcbSetByteSize(&d, conf.DataBits); err != nil {
		return nil, err
	}
	if err := dcbSetStopBits(&d, conf.StopBits); err != nil {
		return nil, err
	}
	if err := dcbSetParity(&d, conf.Parity); err != nil {
		return nil, err
	}

	if err := setCommState(handle, &d); err != nil {
		return nil, err
	}

	ro, err := newOverlapped()
	if err != nil {
		return nil, err
	}

	wo, err := newOverlapped()
	if err != nil {
		return nil, err
	}

	return &port{
		ro: ro, wo: wo,
		handle: handle,
	}, nil
}

func (p *port) Read(b []byte) (int, error) {
	p.mut.RLock()
	defer p.mut.RUnlock()

	if p.handle == windows.InvalidHandle {
		return 0, ErrPortClosed
	}

	var ct windows.CommTimeouts

	if err := windows.GetCommTimeouts(p.handle, &ct); err != nil {
		return 0, err
	}

	timeout := uint32(p.readDeadline.Sub(time.Now()).Milliseconds())
	if timeout <= 0 {
		return 0, os.ErrDeadlineExceeded
	}

	if p.readDeadline.IsZero() {
		ct.ReadIntervalTimeout = maxDWORD
		ct.ReadTotalTimeoutMultiplier = maxDWORD
		ct.ReadTotalTimeoutConstant = 0
	} else {
		ct.ReadIntervalTimeout = 0
		ct.ReadTotalTimeoutMultiplier = 0
		ct.ReadTotalTimeoutConstant = timeout
	}

	if err := windows.SetCommTimeouts(p.handle, &ct); err != nil {
		return 0, err
	}

	var nul uint32
	if err := windows.ReadFile(p.handle, b, &nul, p.ro); err != nil {
		switch err {
		case windows.ERROR_OPERATION_ABORTED:
			return 0, ErrPortClosed
		case windows.ERROR_IO_PENDING:
			// not an error, proceed to wait for completion
		default:
			return 0, err
		}
	}

	var done uint32

	if err := windows.GetOverlappedResult(p.handle, p.ro, &done, true); err != nil {
		switch err {
		case windows.ERROR_OPERATION_ABORTED:
			return 0, ErrPortClosed
		}
		return 0, err
	}

	if !p.readDeadline.IsZero() && int(done) < len(b) {
		return int(done), os.ErrDeadlineExceeded
	}

	return int(done), nil
}

func (p *port) Write(b []byte) (int, error) {
	p.mut.RLock()
	defer p.mut.RUnlock()

	if p.handle == windows.InvalidHandle {
		return 0, ErrPortClosed
	}

	var ct windows.CommTimeouts

	if err := windows.GetCommTimeouts(p.handle, &ct); err != nil {
		return 0, err
	}

	timeout := uint32(p.writeDeadline.Sub(time.Now()).Milliseconds())
	if timeout <= 0 {
		return 0, os.ErrDeadlineExceeded
	}

	if p.writeDeadline.IsZero() {
		ct.WriteTotalTimeoutMultiplier = 0
		ct.WriteTotalTimeoutConstant = 0
	} else {
		ct.WriteTotalTimeoutMultiplier = 0
		ct.WriteTotalTimeoutConstant = timeout
	}

	if err := windows.SetCommTimeouts(p.handle, &ct); err != nil {
		return 0, err
	}

	var nul uint32
	if err := windows.WriteFile(p.handle, b, &nul, p.wo); err != nil {
		switch err {
		case windows.ERROR_OPERATION_ABORTED:
			return 0, ErrPortClosed
		case windows.ERROR_IO_PENDING:
			// not an error, proceed to wait for completion
		}
		return 0, err
	}

	var done uint32
	if err := windows.GetOverlappedResult(p.handle, p.wo, &done, true); err != nil {
		switch err {
		case windows.ERROR_OPERATION_ABORTED:
			return 0, ErrPortClosed
		}
		return 0, err
	}

	if !p.writeDeadline.IsZero() && int(done) < len(b) {
		return int(done), os.ErrDeadlineExceeded
	}

	return int(done), nil
}

func (p *port) Close() error {
	if p.handle == windows.InvalidHandle {
		return nil
	}

	cancelErr := windows.CancelIoEx(p.handle, nil)

	if err := windows.CloseHandle(p.handle); err != nil {
		return err
	}

	p.handle = windows.InvalidHandle

	if cancelErr != windows.ERROR_NOT_FOUND {
		return cancelErr
	}
	return nil
}

func (p *port) SetReadDeadline(t time.Time) error {
	p.readDeadline = t
	return nil
}

func (p *port) SetWriteDeadline(t time.Time) error {
	p.writeDeadline = t
	return nil
}

func dcbInit(d *dcb) {
	d.Flags |= dcbfBinary // enable binary mode

	// disable hardware flow control
	d.Flags &^= dcbfOutxCTSFlow
	d.Flags &^= dcbfOutxDSRFlow
	d.Flags &^= dcbfDTRControl
	d.Flags &^= dcbfRTSControl

	// disable software flow control
	d.Flags &^= dcbfOutX
	d.Flags &^= dcbfInX

	// disable replacement of bytes with parity errors
	d.Flags &^= dcbfErrorChar

	// disable null stripping
	d.Flags &^= dcbfNull
}

func dcbSetBaudRate(d *dcb, baudRate int) error {
	if rate, ok := baudRates[baudRate]; ok {
		d.BaudRate = rate
		return nil
	}

	return fmt.Errorf("unsupported baud rate: %d", baudRate)
}

func dcbSetByteSize(d *dcb, byteSize int) error {
	switch byteSize {
	case 8, 0: // default
		d.ByteSize = 8
	case 7:
		d.ByteSize = 7
	case 6:
		d.ByteSize = 6
	case 5:
		d.ByteSize = 5
	default:
		return fmt.Errorf("unsupported byte size: %v", byteSize)
	}

	return nil
}

func dcbSetStopBits(d *dcb, stopBits StopBits) error {
	switch stopBits {
	case StopBits1, StopBitsNil: // default
		d.StopBits = oneStopBit
	case StopBits2:
		d.StopBits = twoStopBits
	default:
		return fmt.Errorf("unsupported stop bits: %v", stopBits)
	}

	return nil
}

func dcbSetParity(d *dcb, parity Parity) error {
	switch parity {
	case ParityNone:
		d.Flags &^= dcbfParity
		d.Parity = noParity
	case ParityEven, ParityNil: // default
		d.Flags |= dcbfParity
		d.Parity = evenParity
	case ParityOdd:
		d.Flags |= dcbfParity
		d.Parity = oddParity
	default:
		return fmt.Errorf("unsupported parity: %v", parity)
	}

	return nil
}

func newOverlapped() (*windows.Overlapped, error) {
	// https://learn.microsoft.com/en-us/windows/win32/devio/overlapped-operations
	h, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}

	return &windows.Overlapped{HEvent: h}, nil
}
