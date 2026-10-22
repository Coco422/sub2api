package config

import (
	"fmt"
	"net/url"
	"strings"
)

// Empty keeps the shared legacy setting. An override is trusted per-process
// configuration, never an origin derived from a client's Host/forwarded headers.
func ValidateImageRelayOwnerOrigin(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("runtime.image_relay_owner_origin requires an HTTPS origin without credentials, path, query or fragment")
	}
	return nil
}
