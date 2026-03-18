package runtimectx

import (
	"context"

	"github.com/gracesolutions/dns-automatic-updater/internal/containerrt"
)

type Snapshot struct {
	Hostname           string
	NodeID             string
	OS                 string
	Architecture       string
	PublicIPv4         string
	PublicIPv6         string
	RFC1918IPv4        string
	CGNATIPv4          string
	Environment        map[string]string
	InterfaceAddresses map[string][]string

	// ContainerDetector is populated during Resolve and carries the
	// detected Docker/Podman runtime so the reconciliation pipeline can
	// reuse it for container discovery without re-probing sockets.
	ContainerDetector *containerrt.Detector
}

type Resolver interface {
	Resolve(ctx context.Context) (Snapshot, error)
}