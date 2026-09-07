//go:build linux

// SPDX-License-Identifier: AGPL-3.0-only
package tun

import (
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	iffTun    = 0x0001
	iffNoPI   = 0x1000
	tunsetiff = 0x400454ca
)

func Open(name string) (*os.File, error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	var ifr [40]byte
	copy(ifr[:15], []byte(name))
	*(*uint16)(unsafe.Pointer(&ifr[16])) = iffTun | iffNoPI

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(tunsetiff),
		uintptr(unsafe.Pointer(&ifr[0])),
	)
	if errno != 0 {
		_ = unix.Close(fd)
		return nil, errno
	}
	return os.NewFile(uintptr(fd), "/dev/net/tun"), nil
}

func Configure(name, cidr, peerIP string, mtu int) error {
	cmds := [][]string{
		{"ip", "link", "set", "dev", name, "up"},
		{"ip", "addr", "replace", cidr, "dev", name},
	}
	if mtu > 0 {
		cmds = append(cmds, []string{"ip", "link", "set", "dev", name, "mtu", fmt.Sprint(mtu)})
	}
	if peerIP != "" {
		cmds = append(cmds, []string{"ip", "route", "replace", peerIP + "/32", "dev", name})
	}
	for _, argv := range cmds {
		c := exec.Command(argv[0], argv[1:]...)
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", argv, err, string(out))
		}
	}
	// qdisc failure is not fatal on minimal kernels.
	_ = exec.Command("tc", "qdisc", "replace", "dev", name, "root", "fq_codel").Run()
	return nil
}
