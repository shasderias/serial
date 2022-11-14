package serial

import (
	"errors"
	"io"
	"time"
)

type Parity int

const (
	ParityNil Parity = iota
	ParityNone
	ParityOdd
	ParityEven
)

type StopBits int

const (
	StopBitsNil StopBits = iota
	StopBits1
	StopBits2
)

var (
	ErrPortInUse  = errors.New("serial: port in use")
	ErrPortClosed = errors.New("serial: port closed")
)

type Config struct {
	BaudRate int      // default 19200
	DataBits int      // default 8
	StopBits StopBits // default StopBits1
	Parity   Parity   // default ParityEven
}

type Port interface {
	io.ReadWriteCloser
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

func Open(address string, cFns ...func(*Config)) (p Port, err error) {
	conf := Config{}
	for _, cFn := range cFns {
		cFn(&conf)
	}
	return nativeOpen(address, &conf)
}
