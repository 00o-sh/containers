package main

import (
	"testing"

	helpers "github.com/home-operations/containers/tests"
)

func Test(t *testing.T) {
	image := helpers.GetTestImage("ghcr.io/00o-sh/cloudflared-distroless:rolling")
	helpers.RequireCommandSucceeds(t, image, nil, "/usr/bin/cloudflared", "--version")
}
