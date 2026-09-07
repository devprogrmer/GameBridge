// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"path/filepath"
	"testing"
)

func TestCloneStatePreservesSecrets(t *testing.T) {
	in := State{
		Admins:         []Admin{{ID: "a", PasswordHash: "hash", TOTPSecret: "totp"}},
		Nodes:          []Node{{ID: "n", AgentTokenEnc: "agent-secret"}},
		Inbounds:       []Inbound{{ID: "i", RealityPrivateKeyEnc: "reality-secret", ShadowsocksServerPasswordEnc: "ss-secret"}},
		InboundClients: []InboundClient{{ID: "c", CredentialEnc: "credential-secret"}},
		Outbounds:      []Outbound{{ID: "o", PasswordEnc: "outbound-secret"}},
		VPNPeers:       []VPNPeer{{ID: "p", PrivateKeyEnc: "private-secret", ConfigEnc: "config-secret"}},
		Settings:       map[string]string{"site_name": "GameBridge"},
	}
	out := cloneState(in)
	if out.Admins[0].PasswordHash != "hash" || out.Admins[0].TOTPSecret != "totp" {
		t.Fatal("admin secrets were stripped")
	}
	if out.Nodes[0].AgentTokenEnc != "agent-secret" {
		t.Fatal("node token was stripped")
	}
	if out.Inbounds[0].RealityPrivateKeyEnc != "reality-secret" || out.Inbounds[0].ShadowsocksServerPasswordEnc != "ss-secret" {
		t.Fatal("inbound secrets were stripped")
	}
	if out.InboundClients[0].CredentialEnc != "credential-secret" {
		t.Fatal("inbound client secret was stripped")
	}
	if out.Outbounds[0].PasswordEnc != "outbound-secret" {
		t.Fatal("outbound secret was stripped")
	}
	if out.VPNPeers[0].PrivateKeyEnc != "private-secret" || out.VPNPeers[0].ConfigEnc != "config-secret" {
		t.Fatal("wireguard secrets were stripped")
	}
	out.Settings["site_name"] = "changed"
	if in.Settings["site_name"] == "changed" {
		t.Fatal("settings map was not cloned")
	}
}

func TestStorePersistsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Update(func(st *State) error {
		st.Admins = append(st.Admins, Admin{ID: "a", PasswordHash: "hash", TOTPSecret: "totp"})
		st.Nodes = append(st.Nodes, Node{ID: "n", AgentTokenEnc: "agent-secret"})
		st.Inbounds = append(st.Inbounds, Inbound{ID: "i", RealityPrivateKeyEnc: "reality-secret", ShadowsocksServerPasswordEnc: "ss-secret"})
		st.InboundClients = append(st.InboundClients, InboundClient{ID: "c", CredentialEnc: "credential-secret"})
		st.Outbounds = append(st.Outbounds, Outbound{ID: "o", PasswordEnc: "outbound-secret"})
		st.VPNPeers = append(st.VPNPeers, VPNPeer{ID: "p", PrivateKeyEnc: "private-secret", ConfigEnc: "config-secret"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	err = reopened.Read(func(st State) error {
		if st.Admins[0].PasswordHash != "hash" || st.Nodes[0].AgentTokenEnc != "agent-secret" {
			t.Fatal("core secrets did not survive disk roundtrip")
		}
		if st.Inbounds[0].RealityPrivateKeyEnc != "reality-secret" || st.InboundClients[0].CredentialEnc != "credential-secret" {
			t.Fatal("xray secrets did not survive disk roundtrip")
		}
		if st.Outbounds[0].PasswordEnc != "outbound-secret" {
			t.Fatal("outbound secret did not survive disk roundtrip")
		}
		if st.VPNPeers[0].PrivateKeyEnc != "private-secret" || st.VPNPeers[0].ConfigEnc != "config-secret" {
			t.Fatal("wireguard secrets did not survive disk roundtrip")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
