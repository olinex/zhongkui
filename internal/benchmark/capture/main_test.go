package bench

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	PrintResultsTable()
	os.Exit(code)
}
