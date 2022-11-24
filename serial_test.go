package serial_test

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/shasderias/serial"
)

const (
	shortSleepDuration = 10 * time.Millisecond
	longSleepDuration  = 1 * time.Second
)

const (
	baudRate   = 19200
	dataBits   = 8
	parity     = serial.ParityEven
	stopBits   = serial.StopBits1
	testString = "hello world"
)

func TestSanity(t *testing.T) {
	port1, port2 := getTestPorts(t)
	defer port1.Close()
	defer port2.Close()

	n, err := port1.Write([]byte(testString))
	if err != nil {
		t.Fatal(err)
	}
	if n != len(testString) {
		t.Fatalf("%d bytes written; expected %d", n, len(testString))
	}

	buf := make([]byte, 32)

	if err := port2.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	n, err = port2.Read(buf)
	if err != nil && err != os.ErrDeadlineExceeded {
		t.Fatal(err)
	}
	if n != len(testString) {
		t.Fatalf("%d bytes read; want %d", n, len(testString))
	}

	if string(buf[:n]) != testString {
		t.Fatalf("read %q; want %q", string(buf[:n]), testString)
	}

	t.Log(buf)
}

func TestReadDeadline(t *testing.T) {
	portAConnStr, _ := setupLoopbackPorts(t)

	port1, err := serial.Open(portAConnStr, func(c *serial.Config) {
		c.BaudRate = baudRate
		c.DataBits = dataBits
		c.Parity = parity
		c.StopBits = stopBits
	})
	if err != nil {
		t.Fatal(err)
	}
	defer port1.Close()

	alarm := time.After(3 * time.Second)
	done := make(chan struct{})

	go func() {
		buf := make([]byte, 32)
		port1.SetReadDeadline(time.Now().Add(time.Second))
		n, err := port1.Read(buf)
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Logf("got %v; want %v", err, os.ErrDeadlineExceeded)
			t.Fail()
		}
		if n != 0 {
			t.Logf("%d bytes read; want %d", n, 0)
			t.Fail()
		}
		done <- struct{}{}
	}()

	select {
	case <-alarm:
		t.Fatal("got blocking Read(); want Read() to timeout")
	case <-done:
		<-alarm
	}
}

func TestSetReadDeadlineClearsBlocked(t *testing.T) {
	portAConnStr, _ := setupLoopbackPorts(t)

	port1, err := serial.Open(portAConnStr, func(c *serial.Config) {
		c.BaudRate = baudRate
		c.DataBits = dataBits
		c.Parity = parity
		c.StopBits = stopBits
	})
	if err != nil {
		t.Fatal(err)
	}
	defer port1.Close()

	port1.SetReadDeadline(time.Time{})

	blockedReadDone := make(chan struct{})

	go func() {
		buf := make([]byte, 32)
		_, err := port1.Read(buf)
		if err != nil && err != os.ErrDeadlineExceeded {
			t.Log(err)
			t.Fail()
		}
		blockedReadDone <- struct{}{}
	}()

	select {
	case <-time.After(longSleepDuration):
	case <-blockedReadDone:
		t.Fatal("want read to still be blocked")
	}

	port1.SetReadDeadline(time.Now())

	select {
	case <-time.After(longSleepDuration):
		t.Fatal("want read to be unblocked when deadline is set")
	case <-blockedReadDone:
	}
}

func TestRainbow(t *testing.T) {
	port1, port2 := getTestPorts(t)
	defer port1.Close()
	defer port2.Close()

	sendBuf := make([]byte, 256)
	for i := 0; i < len(sendBuf); i++ {
		sendBuf[i] = byte(i)
	}

	recvBuf := make([]byte, 256)

	wg := sync.WaitGroup{}

	wg.Add(2)

	go func() {
		defer wg.Done()
		n, err := port1.Write(sendBuf)
		t.Log("wrote")
		if err != nil {
			t.Log(err)
			t.Fail()
			return
		}
		if n != len(sendBuf) {
			t.Logf("%d bytes written; want %d", n, len(sendBuf))
			t.Fail()
		}
	}()

	go func() {
		defer wg.Done()
		n, err := port2.Read(recvBuf)
		t.Log("read")
		if err != nil && err != os.ErrDeadlineExceeded {
			t.Log(err)
			t.Fail()
		}
		if n != len(recvBuf) {
			t.Logf("%d bytes read; want %d", n, len(recvBuf))
			t.Fail()
		}
	}()

	wg.Wait()
	for i := 0; i < len(recvBuf); i++ {
		if recvBuf[i] != sendBuf[i] {
			t.Log("mismatch at", i)
			t.Fail()
		}
	}
}

func TestLargeRead(t *testing.T) {
	const largeBufSize = 4 * 1024 * 1024 // should be bigger than OS buffers

	port1, port2 := getTestPorts(t)
	defer port1.Close()
	defer port2.Close()

	sendBuf := make([]byte, largeBufSize)
	sendBuf[1024] = 0x13

	recvBuf := make([]byte, largeBufSize)

	wg := sync.WaitGroup{}

	wg.Add(2)

	go func() {
		defer wg.Done()
		n, err := port1.Write(sendBuf)
		t.Log("wrote")
		if err != nil {
			t.Log(err)
			t.Fail()
			return
		}
		if n != largeBufSize {
			t.Logf("%d bytes written; expected %d", n, largeBufSize)
			t.Fail()
		}
	}()

	go func() {
		defer wg.Done()
		n, err := port2.Read(recvBuf)
		t.Log("read")
		if err != nil && err != os.ErrDeadlineExceeded {
			t.Log(err)
			t.Fail()
		}
		if n != largeBufSize {
			t.Logf("%d bytes read; want %d", n, largeBufSize)
			t.Fail()
		}
	}()

	wg.Wait()
	if recvBuf[1024] != 0x13 {
		t.Fatalf("read %x; want %x", recvBuf[1024], 0x13)
	}
}

func TestSerialReadAndCloseConcurrency(t *testing.T) {
	// Run this test with race detector to actually test that
	// the correct multitasking behaviour is happening.
	portPath, _ := setupLoopbackPorts(t)

	port, err := serial.Open(portPath, func(c *serial.Config) {
		c.BaudRate = baudRate
		c.DataBits = dataBits
		c.Parity = parity
		c.StopBits = stopBits
	})
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 100)
	go port.Read(buf)
	// let port.Read to start
	time.Sleep(time.Millisecond * 1)
	port.Close()
}

func TestDoubleCloseIsNoop(t *testing.T) {
	portPath, _ := setupLoopbackPorts(t)

	port, err := serial.Open(portPath, func(c *serial.Config) {
		c.BaudRate = baudRate
		c.DataBits = dataBits
		c.Parity = parity
		c.StopBits = stopBits
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := port.Close(); err != nil {
		t.Fatal(err)
	}
	if err := port.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBlockingRead(t *testing.T) {
	port1, port2 := getTestPorts(t)
	defer port2.Close()

	failAlarm := time.After(longSleepDuration)
	readDone := make(chan struct{})

	buf := make([]byte, 16)
	go func() {
		_, err := port1.Read(buf)
		if err != nil && err != serial.ErrPortClosed {
			t.Log(err)
			t.Fail()
		}
		readDone <- struct{}{}
	}()

	select {
	case <-failAlarm:
		if err := port1.Close(); err != nil {
			t.Log(err)
			t.Fail()
		}
		<-readDone
	case <-readDone:
		t.Fatal("got non-blocking Read(); want blocking Read()")
	}
}

func getTestPorts(t *testing.T) (serial.Port, serial.Port) {
	portAConnStr, portBConnStr := setupLoopbackPorts(t)

	port1, err := serial.Open(portAConnStr, func(c *serial.Config) {
		c.BaudRate = baudRate
		c.DataBits = dataBits
		c.Parity = parity
		c.StopBits = stopBits
	})
	if err != nil {
		t.Fatal(err)
	}

	drain := make([]byte, 1024)

	if err := port1.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := port1.Read(drain); err != nil && err != os.ErrDeadlineExceeded {
		t.Fatal(err)
	}
	if err := port1.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}

	port2, err := serial.Open(portBConnStr, func(c *serial.Config) {
		c.BaudRate = baudRate
		c.DataBits = dataBits
		c.Parity = parity
		c.StopBits = stopBits
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := port2.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := port2.Read(drain); err != nil && err != os.ErrDeadlineExceeded {
		t.Fatal(err)
	}
	if err := port2.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}

	return port1, port2
}

func TestFullDuplex(t *testing.T) {
	portPath, _ := setupLoopbackPorts(t)

	port, err := serial.Open(portPath, func(c *serial.Config) {
		c.BaudRate = baudRate
		c.DataBits = dataBits
		c.Parity = parity
		c.StopBits = stopBits
	})
	if err != nil {
		t.Fatal(err)
	}
	defer port.Close()

	go func() {
		buf := make([]byte, 32)
		_, err := port.Read(buf)
		if err != nil && err != serial.ErrPortClosed {
			t.Log(err)
			t.Fail()
		}
	}()

	time.Sleep(shortSleepDuration)

	wrote := make(chan struct{})
	alarm := time.After(longSleepDuration)

	go func() {
		t.Log("writing")
		if _, err := port.Write([]byte(testString)); err != nil {
			t.Log(err)
			t.Fail()
		}
		t.Log("wrote")
		wrote <- struct{}{}
	}()

	select {
	case <-alarm:
		t.Log("got blocking Write(); want Write() to not block when there is a pending Read()")
		if err := port.Close(); err != nil {
			t.Log(err)
			t.Fail()
		}
		<-wrote
		t.FailNow()
	case <-wrote:
	}

	time.Sleep(longSleepDuration + time.Second)
}

func TestBaudRate(t *testing.T) {
	portPath, _ := setupLoopbackPorts(t)

	port, err := serial.Open(portPath, func(c *serial.Config) {
		c.BaudRate = baudRate
		c.DataBits = dataBits
		c.Parity = parity
		c.StopBits = stopBits
	})
	if err != nil {
		t.Fatal(err)
	}
	defer port.Close()

	sttyCmd := exec.Command("stty", "-F", portPath)
	out, err := sttyCmd.Output()
	if err != nil {
		t.Fatal(err)
	}

	matches := regexp.MustCompile(`speed (\d+) baud`).FindAllSubmatch(out, -1)
	if len(matches) != 1 {
		t.Fatalf("stty output did not contain baud rate or contained more than one baud rate: %s", out)
	}

	if len(matches[0]) != 2 {
		t.Fatalf("want regexp match to contain 2 submatches, got %d: %v", len(matches[0]), matches[0])
	}

	gotBaudRate, err := strconv.Atoi(string(matches[0][1]))
	if err != nil {
		t.Fatalf("error parsing baud rate as integer: %s, %v", matches[0], err)
	}

	if gotBaudRate != baudRate {
		t.Fatalf("got baud rate %d; want %d", gotBaudRate, baudRate)
	}
}
