package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	helpers "github.com/home-operations/containers/tests"
)

func TestVersion(t *testing.T) {
	image := helpers.GetTestImage("ghcr.io/00o-sh/postgres-init-distroless:rolling")
	helpers.RequireCommandSucceeds(t, image, nil, "/usr/bin/postgres-init", "--version")
}

// Full integration: boot a real postgres, run the init image against it
// over a shared network, and assert the role and database actually
// exist afterwards. This is the runtime surface a Wolfi
// postgresql-client bump could break (psql/createuser/createdb flag or
// auth behavior), which --version can't catch.
func TestInitAgainstPostgres(t *testing.T) {
	ctx := t.Context()
	image := helpers.GetTestImage("ghcr.io/00o-sh/postgres-init-distroless:rolling")

	net, err := network.New(ctx)
	require.NoError(t, err)
	testcontainers.CleanupNetwork(t, net)

	pg, err := testcontainers.Run(ctx, "docker.io/library/postgres:18-alpine",
		testcontainers.WithEnv(map[string]string{"POSTGRES_PASSWORD": "supersecret"}),
		network.WithNetwork([]string{"postgres"}, net),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
	)
	testcontainers.CleanupContainer(t, pg)
	require.NoError(t, err)

	initC, err := testcontainers.Run(ctx, image,
		testcontainers.WithEnv(map[string]string{
			"INIT_POSTGRES_HOST":       "postgres",
			"INIT_POSTGRES_SUPER_PASS": "supersecret",
			"INIT_POSTGRES_USER":       "testuser",
			"INIT_POSTGRES_PASS":       "testpass",
			"INIT_POSTGRES_DBNAME":     "testdb",
		}),
		network.WithNetwork(nil, net),
		testcontainers.WithWaitStrategy(wait.ForExit()),
	)
	testcontainers.CleanupContainer(t, initC)
	require.NoError(t, err)

	state, err := initC.State(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, state.ExitCode, "postgres-init should exit 0")

	// Verify via the server's local socket (trust auth) that the work
	// actually happened.
	code, _, err := pg.Exec(ctx, []string{"psql", "-U", "postgres", "-tAc",
		"SELECT 1 FROM pg_roles WHERE rolname='testuser'"})
	require.NoError(t, err)
	require.Equal(t, 0, code)
	code, _, err = pg.Exec(ctx, []string{"psql", "-U", "postgres", "-d", "testdb", "-tAc",
		"SELECT current_database()"})
	require.NoError(t, err)
	require.Equal(t, 0, code, "database testdb should exist and accept connections")
}
