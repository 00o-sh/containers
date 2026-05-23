package main

import (
	"testing"

	"github.com/home-operations/containers/testhelpers"
)

func Test(t *testing.T) {
	image := testhelpers.GetTestImage("ghcr.io/00o-sh/tqm-distroless:rolling")
	testhelpers.TestCommandSucceeds(t, image, nil, "/usr/bin/tqm", "--version")
}
