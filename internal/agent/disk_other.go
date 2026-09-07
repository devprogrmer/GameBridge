// SPDX-License-Identifier: AGPL-3.0-only
//go:build !linux

package agent

func diskUsage(path string) (total, free uint64) {
	return 0, 0
}
