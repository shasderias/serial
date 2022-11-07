//go:build windows

package serial_test

import "testing"

func setupLoopbackPorts(t *testing.T) (string, string) {
	return "COM5", "COM6"
}
