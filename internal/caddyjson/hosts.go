// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package caddyjson

import (
	"fmt"
	"strings"
)

// ValidateHost checks a hostname against Caddy/WebPKI wildcard rules:
// a single '*' is allowed only as the entire leftmost label.
func ValidateHost(host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return fmt.Errorf("host is empty")
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return fmt.Errorf("host %q must not start or end with a dot", host)
	}
	labels := strings.Split(host, ".")
	for i, label := range labels {
		if label == "" {
			return fmt.Errorf("host %q contains an empty label", host)
		}
		if label == "*" {
			if i != 0 {
				return fmt.Errorf("host %q: wildcard is only allowed as the leftmost label", host)
			}
			continue
		}
		if strings.Contains(label, "*") {
			return fmt.Errorf("host %q: partial-label wildcards are not valid", host)
		}
	}
	return nil
}

// ValidateHosts validates every hostname.
func ValidateHosts(hosts []string) error {
	if len(hosts) == 0 {
		return fmt.Errorf("at least one host is required")
	}
	for _, h := range hosts {
		if err := ValidateHost(h); err != nil {
			return err
		}
	}
	return nil
}

// HasWildcard reports whether any host is a wildcard name.
func HasWildcard(hosts []string) bool {
	for _, h := range hosts {
		if strings.HasPrefix(h, "*.") || h == "*" {
			return true
		}
	}
	return false
}

// NestedHandleID returns the @id assigned to a handle nested on a site.
func NestedHandleID(siteName string, index int) string {
	return fmt.Sprintf("%s__h%d", siteName, index)
}

// IsNestedHandleID reports whether id was generated for a nested site handle.
func IsNestedHandleID(siteName, id string) bool {
	return strings.HasPrefix(id, siteName+"__h")
}
