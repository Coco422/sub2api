package urlvalidator

import (
	"errors"
	"net/url"
	"strings"
)

// ValidateHTTPSOrigin is syntax-only and never performs DNS or network I/O.
// Error text never echoes potentially credential-bearing caller input.
func ValidateHTTPSOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("HTTPS origin must not contain credentials, path, query or fragment")
	}
	return nil
}
