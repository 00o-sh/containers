package main

import (
	"testing"

	helpers "github.com/home-operations/containers/tests"
)

func Test(t *testing.T) {
	image := helpers.GetTestImage("ghcr.io/00o-sh/k8s-sidecar-distroless:rolling")
	// The sidecar needs a kube API to do anything: without one, main()
	// logger.fatal-exits non-zero (Wolfi's own recipe smoke greps for
	// CRITICAL for the same reason). Importing the module through the
	// venv interpreter instead proves the interpreter, the app code,
	// and the full dependency tree (kubernetes client et al.) resolve —
	// which is what a Wolfi python/package bump would break.
	helpers.RequireCommandSucceeds(t, image, nil,
		"/usr/share/k8s-sidecar/bin/python", "-c",
		"import sys; sys.path.insert(0, '/usr/share/k8s-sidecar/app'); import sidecar",
	)
}
