//go:build darwin

package machine

import (
	"errors"

	"golang.org/x/sys/unix"
)

func observeAccelerator() (string, error) {
	supported, err := unix.SysctlUint32("kern.hv_support")
	if err != nil {
		return "", err
	}
	if supported != 1 {
		return "", errors.New("hardware virtualization is unavailable")
	}
	return "vz", nil
}
