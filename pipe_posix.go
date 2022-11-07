//go:build linux

package serial

import (
	"fmt"

	"golang.org/x/sys/unix"
)

type pipe struct {
	open   bool
	rd, wr int
}

func newPipe() (*pipe, error) {
	fds := make([]int, 2)
	if err := unix.Pipe(fds); err != nil {
		return nil, err
	}
	return &pipe{true, fds[0], fds[1]}, nil
}

// ReadFD returns the file handle for the read side of the pipe.
func (p *pipe) ReadFD() int {
	if !p.open {
		return -1
	}
	return p.rd
}

// WriteFD returns the file handle for the write side of the pipe.
func (p *pipe) WriteFD() int {
	if !p.open {
		return -1
	}
	return p.wr
}

// Write to the pipe the content of data. Returns the number of bytes written.
func (p *pipe) Write(data []byte) (int, error) {
	if !p.open {
		return 0, fmt.Errorf("pipe not open")
	}
	return unix.Write(p.wr, data)
}

// Read from the pipe into the data array. Returns the number of bytes read.
func (p *pipe) Read(data []byte) (int, error) {
	if !p.open {
		return 0, fmt.Errorf("pipe not open")
	}
	return unix.Read(p.rd, data)
}

// Close the pipe
func (p *pipe) Close() error {
	if !p.open {
		return fmt.Errorf("pipe not open")
	}
	err1 := unix.Close(p.rd)
	err2 := unix.Close(p.wr)
	p.open = false
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}
