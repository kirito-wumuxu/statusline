package node

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strings"
)

type NodeInfo struct {
	NodeID        string `json:"node_id"`
	Hostname      string `json:"hostname"`
	OSType        string `json:"os_type"`
	OSArch        string `json:"os_arch"`
	ClientVersion string `json:"client_version"`
}

func HashMachineID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "fallback-machine-id"
	}
	hash := sha256.Sum256([]byte(raw))
	hexStr := hex.EncodeToString(hash[:])
	if len(hexStr) > 16 {
		hexStr = hexStr[:16]
	}
	return "node_" + hexStr
}

func GetNodeInfo(explicitID, version string) NodeInfo {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-host"
	}

	nodeID := strings.TrimSpace(explicitID)
	if nodeID == "" {
		rawID := readPlatformMachineID()
		if rawID == "" {
			rawID = hostname
		}
		nodeID = HashMachineID(rawID)
	}

	return NodeInfo{
		NodeID:        nodeID,
		Hostname:      hostname,
		OSType:        runtime.GOOS,
		OSArch:        runtime.GOARCH,
		ClientVersion: version,
	}
}
