// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
)

const customXrayConfigMaxBytes = 64 << 10

var customXrayProtocolPattern = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

var customXrayAllowedTopLevel = map[string]bool{
	"protocol":       true,
	"settings":       true,
	"streamSettings": true,
	"mux":            true,
	"sendThrough":    true,
	"targetStrategy": true,
}

var customXrayTargetStrategies = map[string]bool{
	"AsIs":        true,
	"UseIP":       true,
	"UseIPv6v4":   true,
	"UseIPv6":     true,
	"UseIPv4v6":   true,
	"UseIPv4":     true,
	"ForceIP":     true,
	"ForceIPv6v4": true,
	"ForceIPv6":   true,
	"ForceIPv4v6": true,
	"ForceIPv4":   true,
}

func decodeCustomXrayConfig(raw string) (map[string]any, string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", "", errors.New("custom xray config is required")
	}
	if len(raw) > customXrayConfigMaxBytes {
		return nil, "", "", fmt.Errorf("custom xray config exceeds %d bytes", customXrayConfigMaxBytes)
	}

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()

	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, "", "", fmt.Errorf("invalid custom xray JSON: %w", err)
	}
	if obj == nil {
		return nil, "", "", errors.New("custom xray config must be a JSON object")
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, "", "", errors.New("custom xray config must contain exactly one JSON object")
		}
		return nil, "", "", fmt.Errorf("invalid trailing custom xray JSON: %w", err)
	}

	if _, exists := obj["tag"]; exists {
		return nil, "", "", errors.New("custom xray config must not set tag; GameBridge manages outbound tags")
	}
	if _, exists := obj["proxySettings"]; exists {
		return nil, "", "", errors.New("custom xray proxySettings is not allowed in v1; GameBridge must track outbound dependencies")
	}

	for key := range obj {
		if !customXrayAllowedTopLevel[key] {
			return nil, "", "", fmt.Errorf("unsupported custom xray top-level field %q", key)
		}
	}

	protocolValue, ok := obj["protocol"]
	if !ok {
		return nil, "", "", errors.New("custom xray protocol is required")
	}
	protocol, ok := protocolValue.(string)
	if !ok {
		return nil, "", "", errors.New("custom xray protocol must be a string")
	}
	protocol = strings.TrimSpace(protocol)
	if !customXrayProtocolPattern.MatchString(protocol) {
		return nil, "", "", errors.New("custom xray protocol must be lowercase and contain only safe protocol characters")
	}
	obj["protocol"] = protocol

	for _, field := range []string{"settings", "streamSettings", "mux"} {
		if value, exists := obj[field]; exists && value != nil {
			if _, ok := value.(map[string]any); !ok {
				return nil, "", "", fmt.Errorf("custom xray %s must be a JSON object", field)
			}
		}
	}

	if sendThrough, exists := obj["sendThrough"]; exists {
		value, ok := sendThrough.(string)
		if !ok {
			return nil, "", "", errors.New("custom xray sendThrough must be a string")
		}
		value = strings.TrimSpace(value)
		if value == "" {
			delete(obj, "sendThrough")
		} else if net.ParseIP(value) == nil {
			if _, _, err := net.ParseCIDR(value); err != nil {
				return nil, "", "", errors.New("custom xray sendThrough must be an IP address or CIDR")
			}
			obj["sendThrough"] = value
		}
	}

	if strategy, exists := obj["targetStrategy"]; exists {
		value, ok := strategy.(string)
		if !ok || !customXrayTargetStrategies[value] {
			return nil, "", "", errors.New("custom xray targetStrategy is invalid")
		}
	}

	if streamValue, exists := obj["streamSettings"]; exists && streamValue != nil {
		stream := streamValue.(map[string]any)
		if sockoptValue, exists := stream["sockopt"]; exists && sockoptValue != nil {
			sockopt, ok := sockoptValue.(map[string]any)
			if !ok {
				return nil, "", "", errors.New("custom xray streamSettings.sockopt must be a JSON object")
			}
			if dialerProxy, exists := sockopt["dialerProxy"]; exists && dialerProxy != nil {
				value, ok := dialerProxy.(string)
				if !ok {
					return nil, "", "", errors.New("custom xray sockopt.dialerProxy must be a string")
				}
				if strings.TrimSpace(value) != "" {
					return nil, "", "", errors.New("custom xray sockopt.dialerProxy is not allowed in v1; GameBridge must track outbound dependencies")
				}
				delete(sockopt, "dialerProxy")
			}
		}
	}

	canonical, err := json.Marshal(obj)
	if err != nil {
		return nil, "", "", fmt.Errorf("canonicalize custom xray config: %w", err)
	}
	return obj, protocol, string(canonical), nil
}

func customXrayOutboundItem(tag, raw string) (map[string]any, string, error) {
	tag = strings.TrimSpace(tag)
	if !xrayTagPattern.MatchString(tag) || tag == "direct" || tag == "blocked" || tag == "api" {
		return nil, "", errors.New("invalid GameBridge tag for custom xray outbound")
	}
	obj, protocol, _, err := decodeCustomXrayConfig(raw)
	if err != nil {
		return nil, "", err
	}
	obj["tag"] = tag
	return obj, protocol, nil
}
