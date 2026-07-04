package main

import (
	"testing"

	helpers "github.com/home-operations/containers/tests"
)

func Test(t *testing.T) {
	image := helpers.GetTestImage("ghcr.io/00o-sh/kubectl-distroless:rolling")
	// `version --client` exits 0 with the embedded client+go versions
	// without attempting to reach a cluster.
	helpers.RequireCommandSucceeds(t, image, nil, "/usr/bin/kubectl", "version", "--client")
}
