// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateOutboundHealthPort(t *testing.T) {
	for _, port := range []int{outboundHealthPortMin, 31000, outboundHealthPortMax} {
		if err := validateOutboundHealthPort(port); err != nil {
			t.Fatalf("expected port %d to be valid: %v", port, err)
		}
	}
	for _, port := range []int{0, outboundHealthPortMin - 1, outboundHealthPortMax + 1, 65535} {
		if err := validateOutboundHealthPort(port); err == nil {
			t.Fatalf("expected port %d to be rejected", port)
		}
	}
}

func TestBoundedProbeError(t *testing.T) {
	msg := boundedProbeError(errors.New(strings.Repeat("x", 1000)))
	if len(msg) != 512 {
		t.Fatalf("expected bounded error length 512, got %d", len(msg))
	}
}
