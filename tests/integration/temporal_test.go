//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
)

func TestTemporalConnectivity(t *testing.T) {
	hostPort := os.Getenv("TEMPORAL_HOST_PORT")
	if hostPort == "" {
		hostPort = "localhost:7234"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := client.Dial(client.Options{
		HostPort:  hostPort,
		Namespace: "default",
	})
	require.NoError(t, err, "failed to connect to Temporal — is docker-compose.test.yml running?")
	defer c.Close()

	// Verify we can perform health check (basic connectivity check).
	_, err = c.CheckHealth(ctx, &client.CheckHealthRequest{})
	require.NoError(t, err, "Temporal health check failed")
}
