//go:build linux

package serial_test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/shasderias/serial"
)

func startSocat(t *testing.T, args ...string) {
	_, err := exec.LookPath("socat")
	if err != nil {
		t.Skip("socat not found in path")
		return
	}

	cmd := exec.Command("socat", append([]string{"-D"}, args...)...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Logf("error killing socat: %v", err)
		}
		switch err := cmd.Wait().(type) {
		case *exec.ExitError:
			if err.ExitCode() != 130 {
				t.Log(err)
			}
		default:
			t.Log(err)
		}
	})

	// when socat writes to stderr (because of the -D flag), it is ready
	buf := make([]byte, 1024)
	if _, err := stderr.Read(buf); err != nil {
		t.Fatal(err)
	}
}

func setupLoopbackPorts(t *testing.T) (string, string) {
	var (
		tempDir = t.TempDir()

		path1 = path.Join(tempDir, "port1")
		path2 = path.Join(tempDir, "port2")

		port1Def = fmt.Sprintf("pty,raw,echo=0,link=%s", path1)
		port2Def = fmt.Sprintf("pty,raw,echo=0,link=%s", path2)
	)

	startSocat(t, port1Def, port2Def)

	return path1, path2
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

	t.Log(string(out))
}

func TestConfigureTTY(t *testing.T) {
	portPath, _ := setupLoopbackPorts(t)

	setICanonCmd := exec.Command("stty", "-F", portPath, "icanon")
	if err := setICanonCmd.Run(); err != nil {
		t.Fatal(err)
	}

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

	printSTTY(t, portPath)
	sttyCmd := exec.Command("stty", "-F", portPath)
	out, err := sttyCmd.Output()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), "-icanon") {
		t.Fatal("tty is still in canonical mode")
	}
}

func printSTTY(t *testing.T, path string) {
	sttyCmd := exec.Command("stty", "-F", path)
	out, err := sttyCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(out))
}
