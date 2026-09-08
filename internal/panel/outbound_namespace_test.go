// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"strings"
	"testing"
)

func TestValidateOutboundTargetSpecRejectsInternalTagPrefixes(t *testing.T) {
	st := State{
		Nodes: []Node{
			{ID: "node-1", Name: "node-1", Enabled: true},
		},
	}

	for _, tag := range []string{
		"gbm-user-egress",
		"GBM-user-egress",
		"gb-bal-user-egress",
		"GB-BAL-user-egress",
	} {
		err := validateOutboundTargetSpec(&st, "", "user egress", "node-1", tag)
		if err == nil {
			t.Fatalf("expected reserved namespace rejection for tag %q", tag)
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("expected reserved namespace error for %q, got %v", tag, err)
		}
	}
}

func TestValidateOutboundTargetSpecAllowsNonConflictingTags(t *testing.T) {
	st := State{
		Nodes: []Node{
			{ID: "node-1", Name: "node-1", Enabled: true},
		},
	}

	for _, tag := range []string{
		"gaming-egress",
		"gb-health-custom",
		"my-gbm-outbound",
		"my-gb-bal-outbound",
	} {
		if err := validateOutboundTargetSpec(&st, "", "user egress "+tag, "node-1", tag); err != nil {
			t.Fatalf("unexpected rejection for tag %q: %v", tag, err)
		}
	}
}
