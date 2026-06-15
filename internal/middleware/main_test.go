package middleware

import (
	"os"
	"testing"

	"github.com/yxorp/pkg/logger"
)

func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}
