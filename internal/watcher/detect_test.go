package watcher

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

func makeContainerJSON(serviceName string, exposedPorts nat.PortSet, portBindings nat.PortMap) types.ContainerJSON {
	labels := map[string]string{}
	if serviceName != "" {
		labels["com.docker.compose.service"] = serviceName
	}
	info := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &container.Config{Labels: labels},
		NetworkSettings:   &types.NetworkSettings{},
	}
	if exposedPorts != nil {
		info.Config.ExposedPorts = exposedPorts
	}
	if portBindings != nil {
		info.NetworkSettings.NetworkSettingsBase = types.NetworkSettingsBase{
			Ports: portBindings,
		}
	} else {
		info.NetworkSettings.NetworkSettingsBase = types.NetworkSettingsBase{
			Ports: nat.PortMap{},
		}
	}
	return info
}

func TestDetectHTTPPort_Port80(t *testing.T) {
	info := makeContainerJSON("web", nat.PortSet{
		"80/tcp": struct{}{},
	}, nil)
	got := DetectHTTPPort(info)
	if got != "80" {
		t.Errorf("DetectHTTPPort() = %q, want %q", got, "80")
	}
}

func TestDetectHTTPPort_Port3000(t *testing.T) {
	info := makeContainerJSON("app", nat.PortSet{
		"3000/tcp": struct{}{},
	}, nil)
	got := DetectHTTPPort(info)
	if got != "3000" {
		t.Errorf("DetectHTTPPort() = %q, want %q", got, "3000")
	}
}

func TestDetectHTTPPort_MultiplePorts_HighestPriority(t *testing.T) {
	// Port 80 has higher priority than 3000 in httpPorts list
	info := makeContainerJSON("web", nat.PortSet{
		"3000/tcp": struct{}{},
		"80/tcp":   struct{}{},
	}, nil)
	got := DetectHTTPPort(info)
	if got != "80" {
		t.Errorf("DetectHTTPPort() = %q, want %q (80 has higher priority)", got, "80")
	}
}

func TestDetectHTTPPort_SkipPostgresService(t *testing.T) {
	info := makeContainerJSON("postgres", nat.PortSet{
		"5432/tcp": struct{}{},
	}, nil)
	got := DetectHTTPPort(info)
	if got != "" {
		t.Errorf("DetectHTTPPort() = %q, want empty for postgres service", got)
	}
}

func TestDetectHTTPPort_SkipRedisPort(t *testing.T) {
	// Redis port 6379 should be skipped even if service name is not in skipServices
	info := makeContainerJSON("cache", nat.PortSet{
		"6379/tcp": struct{}{},
	}, nil)
	got := DetectHTTPPort(info)
	if got != "" {
		t.Errorf("DetectHTTPPort() = %q, want empty for redis port only", got)
	}
}

func TestDetectHTTPPort_NoExposedPorts(t *testing.T) {
	info := makeContainerJSON("web", nil, nil)
	got := DetectHTTPPort(info)
	if got != "" {
		t.Errorf("DetectHTTPPort() = %q, want empty for no ports", got)
	}
}

func TestDetectHTTPPort_FallbackLowestPort_NumericSort(t *testing.T) {
	// Verify that 9000 < 9999 numerically (not lexicographic "9000" < "999")
	info := makeContainerJSON("app", nat.PortSet{
		"9999/tcp": struct{}{},
		"9000/tcp": struct{}{},
	}, nil)
	got := DetectHTTPPort(info)
	if got != "9000" {
		t.Errorf("DetectHTTPPort() = %q, want %q (numeric sort, not lexicographic)", got, "9000")
	}
}

func TestDetectHTTPPort_PortFromBindings(t *testing.T) {
	info := makeContainerJSON("web", nil, nat.PortMap{
		"8080/tcp": []nat.PortBinding{{HostPort: "8080"}},
	})
	got := DetectHTTPPort(info)
	if got != "8080" {
		t.Errorf("DetectHTTPPort() = %q, want %q", got, "8080")
	}
}

func TestDetectHTTPPort_SkipServiceNames(t *testing.T) {
	for _, svc := range []string{"mysql", "mariadb", "mongo", "mongodb", "redis",
		"memcached", "rabbitmq", "elasticsearch", "zookeeper", "kafka", "mailhog", "mailpit", "minio"} {
		t.Run(svc, func(t *testing.T) {
			info := makeContainerJSON(svc, nat.PortSet{
				"80/tcp": struct{}{},
			}, nil)
			got := DetectHTTPPort(info)
			if got != "" {
				t.Errorf("DetectHTTPPort() = %q, want empty for service %q", got, svc)
			}
		})
	}
}

func TestDetectHTTPPort_UnknownPortNotSkipped(t *testing.T) {
	// A non-skip, non-HTTP port should still be returned as fallback
	info := makeContainerJSON("app", nat.PortSet{
		"4567/tcp": struct{}{},
	}, nil)
	got := DetectHTTPPort(info)
	if got != "4567" {
		t.Errorf("DetectHTTPPort() = %q, want %q (fallback to lowest non-skip)", got, "4567")
	}
}

func TestDetectHTTPPort_CombinedExposedAndBindings(t *testing.T) {
	info := makeContainerJSON("web",
		nat.PortSet{"3000/tcp": struct{}{}},
		nat.PortMap{"80/tcp": []nat.PortBinding{{HostPort: "80"}}},
	)
	got := DetectHTTPPort(info)
	// 80 has higher priority than 3000
	if got != "80" {
		t.Errorf("DetectHTTPPort() = %q, want %q", got, "80")
	}
}
