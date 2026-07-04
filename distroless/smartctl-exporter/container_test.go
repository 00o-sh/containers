package main

import (
	"testing"

	helpers "github.com/home-operations/containers/tests"
)

func Test(t *testing.T) {
	image := helpers.GetTestImage("ghcr.io/00o-sh/smartctl-exporter-distroless:rolling")
	// Mirrors apps/smartctl-exporter's test. Without device access the
	// exporter logs a S.M.A.R.T. read warning but still serves 200 on
	// /metrics (verified against a v0.14.0 build).
	helpers.RequireHTTPEndpoint(t, image, helpers.HTTPTestConfig{
		Port: "9633",
		Path: "/metrics",
	}, nil)
}
