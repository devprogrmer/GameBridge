// SPDX-License-Identifier: AGPL-3.0-only
package system

import (
	"os"
	"os/exec"

	"github.com/devprogrmer/GameBridge/internal/config"
)

func runOK(args ...string) bool {
	if len(args) == 0 {
		return false
	}
	return exec.Command(args[0], args[1:]...).Run() == nil
}

func ensureIptables(table string, rule ...string) {
	check := []string{"iptables"}
	if table != "" {
		check = append(check, "-t", table)
	}
	check = append(check, "-C")
	check = append(check, rule...)
	if runOK(check...) {
		return
	}
	add := []string{"iptables"}
	if table != "" {
		add = append(add, "-t", table)
	}
	add = append(add, "-A")
	add = append(add, rule...)
	_ = exec.Command(add[0], add[1:]...).Run()
}

func EnableNAT(c config.Config) {
	if !c.NAT || c.InternetInterface == "" {
		return
	}
	_ = os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0644)

	ensureIptables("nat", "POSTROUTING", "-o", c.InternetInterface, "-j", "MASQUERADE")
	ensureIptables("", "FORWARD", "-i", c.Interface, "-o", c.InternetInterface, "-j", "ACCEPT")
	ensureIptables("", "FORWARD", "-i", c.InternetInterface, "-o", c.Interface,
		"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT")
}
