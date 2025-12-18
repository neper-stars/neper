package serial

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadKeys(t *testing.T) {
	keys, err := LoadKeys()
	require.NoError(t, err)
	require.NotEmpty(t, keys, "expected at least one key to be loaded")

	// Verify keys have the expected format (8 characters)
	for i, key := range keys[:10] { // Check first 10 keys
		assert.Len(t, key, 8, "key %d should be 8 characters", i)
	}

	// Verify we have a reasonable number of keys (should be around 1.5 million)
	assert.Greater(t, len(keys), 1000000, "expected more than 1 million keys")
}
