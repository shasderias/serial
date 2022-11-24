//go:build linux

package serial

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	tickResolution = 1 // deciseconds
)

type port struct {
	fd int

	mut         sync.RWMutex
	closeSignal *pipe

	closing    bool
	closingMut sync.Mutex

	readDeadline     time.Time
	readDeadlineMut  sync.Mutex
	writeDeadline    time.Time
	writeDeadlineMut sync.Mutex
}

func nativeOpen(path string, conf *Config) (*port, error) {
	fd, err := unix.Open(
		path,
		// https://www.cmrr.umn.edu/~strupp/serial.html#2_5_2
		// https://www.gnu.org/software/libc/manual/html_node/Operating-Modes.html
		// O_NOCTTY: no controlling terminal - prevents input from affecting this process
		// O_NDELAY: don't wait for DCD signal line to be on space voltage
		// O_CLOEXEC: close fd on exec, child processes don't need access to the serial port
		unix.O_RDWR|unix.O_NOCTTY|unix.O_NDELAY|unix.O_CLOEXEC,
		0,
	)
	if err != nil {
		return nil, err
	}

	// O_NDELAY/O_NONBLOCK has overloaded semantics, setting it on Open() means don't block for
	// a "long time" when opening. For serial ports, it may mean waiting for a carrier signal.
	// After the port is opened, the flag determines whether IO is blocking or non-blocking.
	// As we use blocking I/O with timeouts (VTIME/VMIN), we need to clear O_NDELAY/O_NONBLOCK.
	// TODO: what about writes?
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if err != nil {
		return nil, err
	}

	flags &^= unix.O_NDELAY

	_, err = unix.FcntlInt(uintptr(fd), unix.F_SETFD, flags)
	if err != nil {
		return nil, err
	}

	tty, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, fmt.Errorf("error getting termios: %w", err)
	}

	termiosSetRaw(tty)

	if err := termiosSetBaudrate(tty, conf.BaudRate); err != nil {
		return nil, err
	}
	if err := termiosSetCharSize(tty, conf.DataBits); err != nil {
		return nil, err
	}
	if err := termiosSetParity(tty, conf.Parity); err != nil {
		return nil, err
	}
	if err := termiosSetStopBits(tty, conf.StopBits); err != nil {
		return nil, err
	}

	termiosSetTimeout(tty, tickResolution, 0)

	err = unix.IoctlSetTermios(fd, unix.TCSETS, tty)
	if err != nil {
		return nil, fmt.Errorf("error setting termios: %w", err)
	}

	closeSignal, err := newPipe()
	if err != nil {
		return nil, err
	}

	return &port{fd: fd, closeSignal: closeSignal}, nil
}

func (p *port) Read(b []byte) (int, error) {
	p.mut.RLock()
	defer p.mut.RUnlock()

	var read int

	for {
		if p.isClosing() || p.fd == -1 {
			return read, ErrPortClosed
		}
		if p.readDeadlineExpired() {
			return read, os.ErrDeadlineExceeded
		}

		n, err := unix.Read(p.fd, b[read:])
		switch {
		case err == unix.EAGAIN:
			time.Sleep(time.Millisecond)
		case err != nil:
			return n + read, err
		default:
			read += n
		}

		if read == len(b) {
			return read, nil
		}
	}
}

func (p *port) Write(b []byte) (int, error) {
	p.mut.RLock()
	defer p.mut.RUnlock()

	var written int

	for {
		if p.isClosing() || p.fd == -1 {
			return written, ErrPortClosed
		}
		if p.writeDeadlineExpired() {
			return written, os.ErrDeadlineExceeded
		}

		n, err := unix.Write(p.fd, b[written:])
		switch {
		case err == unix.EAGAIN:
			time.Sleep(time.Millisecond)
		case err != nil:
			return n + written, err
		default:
			written += n
		}

		if written == len(b) {
			return written, nil
		}
	}
}

func (p *port) SetReadDeadline(t time.Time) error {
	p.readDeadlineMut.Lock()
	defer p.readDeadlineMut.Unlock()
	p.readDeadline = t
	return nil
}

func (p *port) readDeadlineExpired() bool {
	p.readDeadlineMut.Lock()
	defer p.readDeadlineMut.Unlock()
	return !p.readDeadline.IsZero() && time.Now().After(p.readDeadline)
}

func (p *port) SetWriteDeadline(t time.Time) error {
	p.writeDeadlineMut.Lock()
	defer p.writeDeadlineMut.Unlock()
	p.writeDeadline = t
	return nil
}

func (p *port) writeDeadlineExpired() bool {
	p.writeDeadlineMut.Lock()
	defer p.writeDeadlineMut.Unlock()
	return !p.writeDeadline.IsZero() && time.Now().After(p.writeDeadline)
}

func (p *port) isClosing() bool {
	p.closingMut.Lock()
	defer p.closingMut.Unlock()
	return p.closing
}

func (p *port) Close() error {
	if p.fd == -1 {
		return nil
	}

	p.closingMut.Lock()
	p.closing = true
	p.closingMut.Unlock()

	//p.closeSignal.Write([]byte{0})

	p.mut.Lock()
	defer p.mut.Unlock()

	err := unix.Close(p.fd)
	//p.closeSignal.Close()

	p.fd = -1

	return err
}

func termiosSetRaw(tty *unix.Termios) {
	tty.Cflag |= unix.CREAD   // enable receiver
	tty.Cflag |= unix.CLOCAL  // ignore modem control lines
	tty.Cflag &^= unix.ICANON // disable canonical mode
	tty.Cflag &^= unix.ISIG   // don't interpret INTR, SUSP, DSUSP and QUIT characters
	tty.Cflag &^= unix.ECHO   // don't echo input
	tty.Cflag &^= unix.OPOST  // disable output processing

	// disable echo handling, shouldn't be necessary as ECHO bit is off but just in case
	tty.Lflag &^= unix.ECHOE
	tty.Lflag &^= unix.ECHOK
	tty.Lflag &^= unix.ECHONL
	tty.Lflag &^= unix.ECHOCTL
	tty.Lflag &^= unix.ECHOPRT
	tty.Lflag &^= unix.ECHOKE

	// disable software flow control
	tty.Iflag &^= unix.IXON | unix.IXOFF | unix.IXANY

	// disable special handling of received bytes
	tty.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL
}

func termiosSetBaudrate(tty *unix.Termios, baudRate int) error {
	b, ok := baudRates[baudRate]
	if !ok {
		return fmt.Errorf("unsupported baud rate: %d", baudRate)
	}
	tty.Cflag &^= unix.CBAUD
	tty.Cflag |= b
	return nil
}

func termiosSetCharSize(tty *unix.Termios, charSize int) error {
	s, ok := charSizes[charSize]
	if !ok {
		return fmt.Errorf("unsupported character size: %d", charSize)
	}
	tty.Cflag &^= unix.CSIZE
	tty.Cflag |= s
	return nil
}

func termiosSetStopBits(tty *unix.Termios, stopBits StopBits) error {
	switch stopBits {
	case StopBits1, StopBitsNil:
		tty.Cflag &^= unix.CSTOPB // 1 stop bit
	case StopBits2:
		tty.Cflag |= unix.CSTOPB // 2 stop bits
	default:
		return fmt.Errorf("unsupported stop bits: %v", stopBits)
	}
	return nil
}

func termiosSetParity(tty *unix.Termios, parity Parity) error {
	switch parity {
	case ParityNone:
		tty.Cflag &^= unix.PARENB // disable parity
	case ParityEven, ParityNil:
		tty.Cflag |= unix.PARENB  // enable parity
		tty.Cflag &^= unix.PARODD // even parity
		tty.Iflag |= unix.INPCK   // check parity
		tty.Iflag &^= unix.IGNPAR // don't ignore framing errors and parity errors
	case ParityOdd:
		tty.Cflag |= unix.PARENB  // enable parity
		tty.Cflag |= unix.PARODD  // odd parity
		tty.Iflag |= unix.INPCK   // check parity
		tty.Iflag &^= unix.IGNPAR // don't ignore framing errors and parity errors
	default:
		return fmt.Errorf("unsupported parity: %v", parity)
	}
	return nil
}

func termiosSetTimeout(tty *unix.Termios, vtime, vmin byte) {
	tty.Cc[unix.VTIME] = vtime
	tty.Cc[unix.VMIN] = vmin
}
