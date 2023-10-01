package tests

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// here we can set up whatever system we need before our server runs (like a test S3)
	code := m.Run()
	// here we can shut down whatever system needs a shutdown
	os.Exit(code)
}
