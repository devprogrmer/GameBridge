// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"fmt"
	"net/http"
)

type nativeWireGuardSpec struct {
	Interface           string   `json:"interface"`
	Address             string   `json:"address"`
	PrivateKey          string   `json:"private_key"`
	PeerPublicKey       string   `json:"peer_public_key"`
	Endpoint            string   `json:"endpoint"`
	AllowedIPs          []string `json:"allowed_ips"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
	MTU                 int      `json:"mtu"`
}

func (s *Server) desiredWireGuardOutbounds(nodeID string) (Node, []nativeWireGuardSpec, error) {
	var node Node
	var specs []nativeWireGuardSpec
	err := s.store.Read(func(st State) error {
		n := findNode(&st, nodeID)
		if n == nil {
			return errors.New("node not found")
		}
		node = *n
		for _, out := range st.Outbounds {
			if out.NodeID != nodeID || !out.Enabled || out.Protocol != "wireguard" {
				continue
			}
			privateKey, err := s.crypt.Open(out.PasswordEnc)
			if err != nil {
				return fmt.Errorf("decrypt WireGuard outbound %s private key: %w", out.Tag, err)
			}
			specs = append(specs, nativeWireGuardSpec{
				Interface:           out.WireGuardInterface,
				Address:             out.WireGuardAddress,
				PrivateKey:          privateKey,
				PeerPublicKey:       out.WireGuardPeerPublicKey,
				Endpoint:            fmt.Sprintf("%s:%d", out.Address, out.Port),
				AllowedIPs:          append([]string(nil), out.WireGuardAllowedIPs...),
				PersistentKeepalive: out.WireGuardKeepalive,
				MTU:                 out.WireGuardMTU,
			})
		}
		return nil
	})
	return node, specs, err
}

func (s *Server) syncWireGuardOutbounds(nodeID string) error {
	node, specs, err := s.desiredWireGuardOutbounds(nodeID)
	if err != nil {
		return err
	}
	payload := map[string]any{"outbounds": specs}
	if err := s.agentJSON(node, http.MethodPost, "/v1/wireguard/outbounds/sync", payload, nil); err != nil {
		return fmt.Errorf("sync native WireGuard outbounds: %w", err)
	}
	return nil
}

func (s *Server) deployOutboundNode(nodeID string) error {
	if err := s.syncWireGuardOutbounds(nodeID); err != nil {
		return err
	}
	if err := s.deployXrayNode(nodeID); err != nil {
		return fmt.Errorf("deploy Xray after native outbound sync: %w", err)
	}
	return nil
}
