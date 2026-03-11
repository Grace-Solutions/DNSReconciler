package runtimectx

import "context"

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
}

type Resolver interface {
	Resolve(ctx context.Context) (Snapshot, error)
}