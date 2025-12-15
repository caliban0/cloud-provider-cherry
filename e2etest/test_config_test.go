package e2etest

import (
	"os"
	"sync"
	"testing"

	"github.com/cherryservers/cloud-provider-cherry-tests/cherry"
)

// TODO Cherry client should be an interface that's defined here.

// TODO: rename to cherryClient, once refactor done
var getCherryClient = func() func(t *testing.T) cherry.Client {
	var (
		once sync.Once
		c    cherry.Client
		err  error
	)
	return func(t *testing.T) cherry.Client {
		t.Helper()
		once.Do(func() {
			apiToken, ok := os.LookupEnv(apiTokenVar)
			if !ok {
				t.Fatalf("%s not set", apiTokenVar)
			}
			c, err = cherry.NewClient(apiToken)
			if err != nil {
				t.Fatalf("failed to build cherry API client: %v", err)
			}
		})
		return c
	}
}()
