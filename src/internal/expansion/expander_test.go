package expansion

import (
	"testing"

	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
)

func TestExpand_BasicSubstitution(t *testing.T) {
	ctx := Context{"HOSTNAME": "web01", "ZONE": "example.com"}
	r := Expand("${HOSTNAME}.${ZONE}", ctx)
	if r.Value != "web01.example.com" {
		t.Errorf("got %q; want %q", r.Value, "web01.example.com")
	}
	if len(r.Unresolved) != 0 {
		t.Errorf("expected no unresolved; got %v", r.Unresolved)
	}
}

func TestExpand_Unresolved(t *testing.T) {
	ctx := Context{"HOSTNAME": "web01"}
	r := Expand("${HOSTNAME}.${ZONE}", ctx)
	if r.Value != "web01.${ZONE}" {
		t.Errorf("got %q; want %q", r.Value, "web01.${ZONE}")
	}
	if len(r.Unresolved) != 1 || r.Unresolved[0] != "ZONE" {
		t.Errorf("expected unresolved=[ZONE]; got %v", r.Unresolved)
	}
}

func TestExpand_NoVariables(t *testing.T) {
	r := Expand("static.example.com", Context{})
	if r.Value != "static.example.com" {
		t.Errorf("got %q", r.Value)
	}
	if len(r.Unresolved) != 0 {
		t.Errorf("expected no unresolved; got %v", r.Unresolved)
	}
}

func TestMustExpand_ErrorOnUnresolved(t *testing.T) {
	ctx := Context{}
	_, err := MustExpand("${MISSING}", ctx)
	if err == nil {
		t.Fatal("expected error on unresolved variable")
	}
}

func TestMustExpand_Success(t *testing.T) {
	ctx := Context{"OS": "linux"}
	val, err := MustExpand("os=${OS}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "os=linux" {
		t.Errorf("got %q; want %q", val, "os=linux")
	}
}

func TestExpandAll(t *testing.T) {
	ctx := Context{"A": "1", "B": "2"}
	out, unresolved := ExpandAll([]string{"${A}", "${B}", "${C}"}, ctx)
	if out[0] != "1" || out[1] != "2" || out[2] != "${C}" {
		t.Errorf("unexpected output: %v", out)
	}
	if len(unresolved) != 1 || unresolved[0] != "C" {
		t.Errorf("expected unresolved=[C]; got %v", unresolved)
	}
}

func TestExpandMap(t *testing.T) {
	ctx := Context{"HOST": "web01"}
	out, unresolved := ExpandMap(map[string]string{"name": "${HOST}"}, ctx)
	if out["name"] != "web01" {
		t.Errorf("got %q; want %q", out["name"], "web01")
	}
	if len(unresolved) != 0 {
		t.Errorf("expected no unresolved; got %v", unresolved)
	}
}

func TestBuildContext_AllVarsPresent(t *testing.T) {
	snap := runtimectx.Snapshot{
		Hostname:     "h1",
		NodeID:       "n1",
		OS:           "linux",
		Architecture: "amd64",
		PublicIPv4:   "1.2.3.4",
		PublicIPv6:   "::1",
		RFC1918IPv4:  "10.0.0.1",
		CGNATIPv4:    "100.64.0.1",
	}
	rv := RecordVars{
		SelectedIPv4: "1.2.3.4",
		SelectedIPv6: "::1",
		ServiceName:  "svc",
		StackName:    "stack",
		Zone:         "example.com",
		RecordID:     "rec1",
	}
	ctx := BuildContext(snap, rv)
	required := []string{
		"HOSTNAME", "NODE_ID", "OS", "ARCH",
		"PUBLIC_IPV4", "PUBLIC_IPV6", "RFC1918_IPV4", "CGNAT_IPV4",
		"SELECTED_IPV4", "SELECTED_IPV6", "SERVICE_NAME", "STACK_NAME",
		"ZONE", "RECORD_ID",
	}
	for _, key := range required {
		if _, ok := ctx[key]; !ok {
			t.Errorf("missing required variable: %s", key)
		}
	}
}

