package containerrt

import (
	"os"
	"strings"
)

// --- Docker/Podman API response structures ---

type dockerNetwork struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	IPAM   struct {
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	} `json:"IPAM"`
}

type dockerContainer struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	State           string            `json:"State"`
	Labels          map[string]string `json:"Labels"`
	NetworkSettings struct {
		Networks map[string]dockerContainerNetwork `json:"Networks"`
	} `json:"NetworkSettings"`
}

type dockerContainerNetwork struct {
	NetworkID  string `json:"NetworkID"`
	IPAddress  string `json:"IPAddress"`
	MacAddress string `json:"MacAddress"`
	Gateway    string `json:"Gateway"`
}

// --- Helpers ---

// isSocket returns true if path exists and is a Unix domain socket.
func isSocket(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().Type()&os.ModeSocket != 0
}

// cleanContainerName strips the leading "/" from Docker container names
// and returns the first name if multiple are present.
func cleanContainerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

