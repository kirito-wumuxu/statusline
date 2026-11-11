package node

import (
	"strings"
	"testing"
)

func TestHashMachineID(t *testing.T) {
	raw := "c7d23a1f-4b82-4f33-b91b-8b548bce91c5"
	hashed := HashMachineID(raw)
	if !strings.HasPrefix(hashed, "node_") {
		t.Fatalf("expected prefix node_, got %s", hashed)
	}
	if len(hashed) != 5+16 { // "node_" + 16 chars = 21
		t.Fatalf("expected length 21, got %d (%s)", len(hashed), hashed)
	}
	// Verify deterministic
	if HashMachineID(raw) != hashed {
		t.Fatalf("expected deterministic hash")
	}
}

func TestGetNodeInfoExplicit(t *testing.T) {
	info := GetNodeInfo("my-node-01", "1.0.0")
	if info.NodeID != "my-node-01" {
		t.Fatalf("expected my-node-01, got %s", info.NodeID)
	}
	if info.OSType == "" || info.OSArch == "" || info.Hostname == "" {
		t.Fatalf("expected non-empty OS details, got %+v", info)
	}
	if info.ClientVersion != "1.0.0" {
		t.Fatalf("expected version 1.0.0, got %s", info.ClientVersion)
	}
}

func TestGetNodeInfoAutomatic(t *testing.T) {
	info := GetNodeInfo("", "1.0.0")
	if info.NodeID == "" {
		t.Fatalf("expected non-empty automatic node ID")
	}
	if !strings.HasPrefix(info.NodeID, "node_") {
		t.Fatalf("expected automatic node ID to start with node_, got %s", info.NodeID)
	}
}
