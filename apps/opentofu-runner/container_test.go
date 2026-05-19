package main

import (
	"testing"

	"github.com/home-operations/containers/testhelpers"
)

func Test(t *testing.T) {
	image := testhelpers.GetTestImage("ghcr.io/home-operations/opentofu-runner:rolling")
	testhelpers.TestFileExists(t, image, "/usr/local/bin/terraform", nil)
}
