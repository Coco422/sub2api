package urlvalidator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateHTTPSOrigin(t *testing.T) {
	for _, value := range []string{"https://node-a.example.invalid", "https://node-a.example.invalid/", "https://node-a.example.invalid:8443"} {
		require.NoError(t, ValidateHTTPSOrigin(value))
	}
	for _, value := range []string{"", "http://node-a.example.invalid", "https://user:private@node-a.example.invalid", "https://node-a.example.invalid/path", "https://node-a.example.invalid/%2f", "https://node-a.example.invalid?secret=private", "https://node-a.example.invalid#"} {
		err := ValidateHTTPSOrigin(value)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private")
	}
}
