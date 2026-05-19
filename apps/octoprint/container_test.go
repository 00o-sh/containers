package main

import (
	"testing"

	"github.com/home-operations/containers/testhelpers"
)

func Test(t *testing.T) {
	image := testhelpers.GetTestImage("ghcr.io/home-operations/octoprint:rolling")
	testhelpers.TestHTTPEndpoint(t, image, testhelpers.HTTPTestConfig{Port: "5000"}, nil)
}
