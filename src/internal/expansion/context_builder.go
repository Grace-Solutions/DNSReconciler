package expansion

import (
	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
)

// RecordVars holds per-record values needed for variable expansion.
type RecordVars struct {
	SelectedIPv4 string
	SelectedIPv6 string
	ServiceName  string
	StackName    string
	Zone         string
	RecordID     string
}

// BuildContext creates a full expansion Context from the runtime snapshot
// and per-record variables. This implements the full §19.1 variable set.
func BuildContext(snap runtimectx.Snapshot, rv RecordVars) Context {
	return Context{
		"HOSTNAME":      snap.Hostname,
		"NODE_ID":       snap.NodeID,
		"OS":            snap.OS,
		"ARCH":          snap.Architecture,
		"PUBLIC_IPV4":   snap.PublicIPv4,
		"PUBLIC_IPV6":   snap.PublicIPv6,
		"RFC1918_IPV4":  snap.RFC1918IPv4,
		"CGNAT_IPV4":    snap.CGNATIPv4,
		"SELECTED_IPV4": rv.SelectedIPv4,
		"SELECTED_IPV6": rv.SelectedIPv6,
		"SERVICE_NAME":  rv.ServiceName,
		"STACK_NAME":    rv.StackName,
		"ZONE":          rv.Zone,
		"RECORD_ID":     rv.RecordID,
	}
}

