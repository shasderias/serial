//go:build linux

package serial_test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"
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
