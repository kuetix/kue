package integration

import (
	"os"
	"testing"

	"github.com/kuetix/engine/boot"
	"github.com/kuetix/kue/tests/testutil"
)

func TestMain(m *testing.M) {
	testutil.EnableModules()
	boot.DependencyInjection() // populate the DI FactoryContainer
	os.Exit(m.Run())
}
