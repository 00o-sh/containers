package main

import (
	"testing"

	"github.com/home-operations/containers/testhelpers"
)

func TestVersion(t *testing.T) {
	image := testhelpers.GetTestImage("ghcr.io/00o-sh/kopia-distroless:rolling")
	testhelpers.TestCommandSucceeds(t, image, nil, "/usr/bin/kopia", "--version")
}

// The image's default cmd is server mode with the embedded web UI
// (parity with apps/kopia's KOPIA_WEB_ENABLED=true default). With no
// server credentials configured, kopia answers 401 at / (basic-auth
// gate — verified empirically against v0.23.0; same behavior as the
// apps/ flavor's default invocation, where users set credentials at
// runtime). Asserting the 401 proves the server boots, binds 51515,
// and the UI/auth stack responds — with an empty /config.
func TestServerUI(t *testing.T) {
	image := testhelpers.GetTestImage("ghcr.io/00o-sh/kopia-distroless:rolling")
	testhelpers.TestHTTPEndpoint(t, image, testhelpers.HTTPTestConfig{Port: "51515", StatusCode: 401}, nil)
}
