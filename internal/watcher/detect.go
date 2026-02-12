package watcher

import (
	"strconv"

	"github.com/docker/docker/api/types"
)

// Known HTTP ports in priority order.
var httpPorts = []string{"80", "443", "3000", "3001", "4000", "5000", "5173", "8000", "8080", "8443"}

// Known non-HTTP ports to skip.
var skipPorts = map[string]bool{
	"5432":  true, // postgres
	"3306":  true, // mysql
	"27017": true, // mongo
	"6379":  true, // redis
	"5672":  true, // rabbitmq
	"15672": true, // rabbitmq management (actually HTTP but not a primary service)
	"9200":  true, // elasticsearch
	"9300":  true, // elasticsearch transport
	"2181":  true, // zookeeper
	"9092":  true, // kafka
	"11211": true, // memcached
}

// Known non-HTTP service names to skip.
var skipServices = map[string]bool{
	"postgres":      true,
	"postgresql":    true,
	"mysql":         true,
	"mariadb":       true,
	"mongo":         true,
	"mongodb":       true,
	"redis":         true,
	"memcached":     true,
	"rabbitmq":      true,
	"elasticsearch": true,
	"zookeeper":     true,
	"kafka":         true,
	"mailhog":       true,
	"mailpit":       true,
	"minio":         true,
}

// DetectHTTPPort inspects a container and returns the likely HTTP port, or "" if none found.
func DetectHTTPPort(info types.ContainerJSON) string {
	// Check service name - skip known non-HTTP services
	serviceName := info.Config.Labels["com.docker.compose.service"]
	if skipServices[serviceName] {
		return ""
	}

	// Collect all exposed ports
	exposedPorts := make(map[string]bool)

	// From container config (EXPOSE in Dockerfile)
	if info.Config != nil {
		for port := range info.Config.ExposedPorts {
			exposedPorts[port.Port()] = true
		}
	}

	// From host port bindings
	if info.NetworkSettings != nil {
		for port := range info.NetworkSettings.Ports {
			exposedPorts[port.Port()] = true
		}
	}

	if len(exposedPorts) == 0 {
		return ""
	}

	// Check known HTTP ports in priority order
	for _, p := range httpPorts {
		if exposedPorts[p] {
			return p
		}
	}

	// Find lowest port (numerically) that isn't in skip list
	lowestNum := 0
	var lowest string
	for port := range exposedPorts {
		if skipPorts[port] {
			continue
		}
		num, err := strconv.Atoi(port)
		if err != nil {
			continue
		}
		if lowest == "" || num < lowestNum {
			lowest = port
			lowestNum = num
		}
	}

	return lowest
}
