// SPDX-License-Identifier: AGPL-3.0-only
package agent

import "testing"

func TestNormalizeXrayUserStats(t *testing.T) {
	raw := []byte(`{
	  "stat": [
	    {"name":"user>>>gb:alice>>>traffic>>>uplink","value":"100"},
	    {"name":"user>>>gb:alice>>>traffic>>>downlink","value":"200"},
	    {"name":"user>>>gb:bob>>>traffic>>>uplink","value":50},
	    {"name":"inbound>>>x>>>traffic>>>uplink","value":"999"}
	  ]
	}`)
	out, err := normalizeXrayUserStats(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 users, got %d", len(out))
	}
	if out[0].Email != "gb:alice" || out[0].Uplink != 100 || out[0].Downlink != 200 {
		t.Fatalf("unexpected alice stats: %#v", out[0])
	}
	if out[1].Email != "gb:bob" || out[1].Uplink != 50 {
		t.Fatalf("unexpected bob stats: %#v", out[1])
	}
}

func TestParseWGRuntimeDump(t *testing.T) {
	dump := "wg0 pub preshared 198.51.100.1:51820 10.0.0.2/32 1700000000 123 456 25\n"
	peers := parseWGRuntimeDump(dump)
	if len(peers) != 1 {
		t.Fatalf("expected one peer, got %d", len(peers))
	}
	if peers[0].LatestHandshake != 1700000000 || peers[0].RXBytes != 123 || peers[0].TXBytes != 456 {
		t.Fatalf("unexpected peer: %+v", peers[0])
	}
}
