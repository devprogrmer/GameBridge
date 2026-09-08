// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
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

type nativeTorSpec struct {
	Name      string `json:"name"`
	SocksPort int    `json:"socks_port"`
}

func (s *Server) desiredTorOutbounds(nodeID string) (Node, []nativeTorSpec, error) {
	var node Node
	var specs []nativeTorSpec
	err := s.store.Read(func(st State) error {
		n := findNode(&st, nodeID)
		if n == nil {
			return errors.New("node not found")
		}
		node = *n
		for _, out := range st.Outbounds {
			if out.NodeID != nodeID || !out.Enabled || out.Protocol != "tor" {
				continue
			}
			specs = append(specs, nativeTorSpec{
				Name:      out.Tag,
				SocksPort: out.TorSOCKSPort,
			})
		}
		return nil
	})
	return node, specs, err
}

func (s *Server) syncTorOutbounds(nodeID string) error {
	node, specs, err := s.desiredTorOutbounds(nodeID)
	if err != nil {
		return err
	}
	payload := map[string]any{"outbounds": specs}
	if err := s.agentJSON(node, http.MethodPost, "/v1/tor/outbounds/sync", payload, nil); err != nil {
		return fmt.Errorf("sync native Tor outbounds: %w", err)
	}
	return nil
}

type openVPNCredential struct {
	Profile  string `json:"profile"`
	Password string `json:"password,omitempty"`
}

func encodeOpenVPNCredential(profile, password string) (string, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return "", errors.New("openvpn profile is required")
	}
	b, err := json.Marshal(openVPNCredential{Profile: profile, Password: password})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeOpenVPNCredential(raw string) (openVPNCredential, error) {
	var out openVPNCredential
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return out, fmt.Errorf("decode openvpn credential: %w", err)
	}
	out.Profile = strings.TrimSpace(out.Profile)
	if out.Profile == "" {
		return out, errors.New("openvpn credential has an empty profile")
	}
	return out, nil
}

type nativeOpenVPNSpec struct {
	Name         string `json:"name"`
	Interface    string `json:"interface"`
	RoutingTable int    `json:"routing_table"`
	Mark         int    `json:"mark"`
	Profile      string `json:"profile"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

func (s *Server) desiredOpenVPNOutbounds(nodeID string) (Node, []nativeOpenVPNSpec, error) {
	var node Node
	var specs []nativeOpenVPNSpec
	err := s.store.Read(func(st State) error {
		n := findNode(&st, nodeID)
		if n == nil {
			return errors.New("node not found")
		}
		node = *n

		seenInterfaces := map[string]bool{}
		seenTables := map[int]bool{}
		seenMarks := map[int]bool{}

		for _, out := range st.Outbounds {
			if out.NodeID != nodeID || !out.Enabled || out.Protocol != "openvpn" {
				continue
			}
			if seenInterfaces[out.OpenVPNInterface] {
				return fmt.Errorf("duplicate openvpn interface %s on node", out.OpenVPNInterface)
			}
			if seenTables[out.OpenVPNRoutingTable] {
				return fmt.Errorf("duplicate openvpn routing table %d on node", out.OpenVPNRoutingTable)
			}
			if seenMarks[out.OpenVPNMark] {
				return fmt.Errorf("duplicate openvpn mark %d on node", out.OpenVPNMark)
			}
			seenInterfaces[out.OpenVPNInterface] = true
			seenTables[out.OpenVPNRoutingTable] = true
			seenMarks[out.OpenVPNMark] = true

			plain, err := s.crypt.Open(out.PasswordEnc)
			if err != nil {
				return fmt.Errorf("decrypt openvpn outbound %s profile: %w", out.Tag, err)
			}
			cred, err := decodeOpenVPNCredential(plain)
			if err != nil {
				return fmt.Errorf("openvpn outbound %s credential: %w", out.Tag, err)
			}
			specs = append(specs, nativeOpenVPNSpec{
				Name:         out.Tag,
				Interface:    out.OpenVPNInterface,
				RoutingTable: out.OpenVPNRoutingTable,
				Mark:         out.OpenVPNMark,
				Profile:      cred.Profile,
				Username:     out.Username,
				Password:     cred.Password,
			})
		}
		return nil
	})
	return node, specs, err
}

func (s *Server) syncOpenVPNOutbounds(nodeID string) error {
	node, specs, err := s.desiredOpenVPNOutbounds(nodeID)
	if err != nil {
		return err
	}
	payload := map[string]any{"outbounds": specs}
	if err := s.agentJSON(node, http.MethodPost, "/v1/openvpn/outbounds/sync", payload, nil); err != nil {
		return fmt.Errorf("sync native openvpn outbounds: %w", err)
	}
	return nil
}

func (s *Server) deployOutboundNode(nodeID string) error {
	if err := s.syncWireGuardOutbounds(nodeID); err != nil {
		return err
	}
	if err := s.syncTorOutbounds(nodeID); err != nil {
		return err
	}
	if err := s.syncOpenVPNOutbounds(nodeID); err != nil {
		return err
	}
	if err := s.deployXrayNode(nodeID); err != nil {
		return fmt.Errorf("deploy Xray after native outbound sync: %w", err)
	}
	return nil
}
