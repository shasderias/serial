package serial_test

import (
	"errors"
	"os"
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

	port1, err := serial.Open(portAConnStr, &serial.Config{
		BaudRate: baudRate,
		DataBits: dataBits,
		Parity:   parity,
		StopBits: stopBits,
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

func TestSerialReadAndCloseConcurrency(t *testing.T) {
	// Run this test with race detector to actually test that
	// the correct multitasking behaviour is happening.
	portPath, _ := setupLoopbackPorts(t)

	port, err := serial.Open(portPath, &serial.Config{
		BaudRate: baudRate,
		DataBits: dataBits,
		Parity:   parity,
		StopBits: stopBits,
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

	port, err := serial.Open(portPath, &serial.Config{
		BaudRate: baudRate,
		DataBits: dataBits,
		Parity:   parity,
		StopBits: stopBits,
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

	port1, err := serial.Open(portAConnStr, &serial.Config{
		BaudRate: baudRate,
		DataBits: dataBits,
		StopBits: stopBits,
		Parity:   parity,
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

	port2, err := serial.Open(portBConnStr, &serial.Config{
		BaudRate: baudRate,
		DataBits: dataBits,
		StopBits: stopBits,
		Parity:   parity,
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

	port, err := serial.Open(portPath, &serial.Config{
		BaudRate: baudRate,
		DataBits: dataBits,
		Parity:   parity,
		StopBits: stopBits,
	})
	if err != nil {
		t.Fatal(err)
	}

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
