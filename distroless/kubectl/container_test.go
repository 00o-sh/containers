package main

import (
	"testing"

	"github.com/home-operations/containers/testhelpers"
)

func Test(t *testing.T) {
	image := testhelpers.GetTestImage("ghcr.io/00o-sh/kubectl-distroless:rolling")
	// `version --client` exits 0 with the embedded client+go versions
	// without attempting to reach a cluster.
	testhelpers.TestCommandSucceeds(t, image, nil, "/usr/bin/kubectl", "version", "--client")
}
