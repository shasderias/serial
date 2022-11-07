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

func Open(address string, c *Config) (p Port, err error) {
	return nativeOpen(address, c)
}
