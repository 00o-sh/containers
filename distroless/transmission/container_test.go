package main

import (
	"testing"

	helpers "github.com/home-operations/containers/tests"
)

// Default boot: no TRANSMISSION__* env, so the shim skips rendering and
// the daemon starts with its own defaults — RPC answers 403 to
// non-whitelisted (non-local) clients, same assertion as the
// apps/transmission test.
func TestDefaultBoot(t *testing.T) {
	image := helpers.GetTestImage("ghcr.io/00o-sh/transmission-distroless:rolling")
	helpers.RequireHTTPEndpoint(t, image, helpers.HTTPTestConfig{
		Port:       "9091",
		StatusCode: 403,
	}, nil)
}

// Env-templated boot: setting TRANSMISSION__* vars makes the shim
// render /config/settings.json. Disabling both whitelists must let the
// test client reach the bundled web UI — this proves the rendered
// config is valid AND actually loaded (the old apps/ template produced
// invalid JSON, so this path never worked there), and that the web UI
// assets are shipped.
func TestEnvConfigAndWebUI(t *testing.T) {
	image := helpers.GetTestImage("ghcr.io/00o-sh/transmission-distroless:rolling")
	helpers.RequireHTTPEndpoint(t, image, helpers.HTTPTestConfig{
		Port:       "9091",
		Path:       "/transmission/web/",
		StatusCode: 200,
	}, &helpers.ContainerConfig{Env: map[string]string{
		"TRANSMISSION__RPC_WHITELIST_ENABLED":      "false",
		"TRANSMISSION__RPC_HOST_WHITELIST_ENABLED": "false",
	}})
}
