package watcher

import (
	"strings"
	"sync"
	"testing"
)

func TestGenerateCaddyfile_EmptyRoutes(t *testing.T) {
	routes := NewActiveRoutes()
	got, err := GenerateCaddyfile(routes)
	if err != nil {
		t.Fatalf("GenerateCaddyfile() error = %v", err)
	}
	if !strings.Contains(got, "local_certs") {
		t.Error("expected local_certs in output")
	}
	if !strings.Contains(got, "skip_install_trust") {
		t.Error("expected skip_install_trust in output")
	}
	// Should not contain any reverse_proxy directives
	if strings.Contains(got, "reverse_proxy") {
		t.Error("expected no reverse_proxy in empty routes output")
	}
}

func TestGenerateCaddyfile_SingleRoute(t *testing.T) {
	routes := NewActiveRoutes()
	routes.Add("c1", &Route{
		Hostname:      "app.localhost",
		ContainerName: "myapp-web-1",
		Port:          "3000",
	})
	got, err := GenerateCaddyfile(routes)
	if err != nil {
		t.Fatalf("GenerateCaddyfile() error = %v", err)
	}
	if !strings.Contains(got, "app.localhost {") {
		t.Error("expected hostname block")
	}
	if !strings.Contains(got, "tls internal") {
		t.Error("expected tls internal")
	}
	if !strings.Contains(got, "reverse_proxy myapp-web-1:3000") {
		t.Error("expected reverse_proxy directive")
	}
}

func TestGenerateCaddyfile_MultipleRoutes_Sorted(t *testing.T) {
	routes := NewActiveRoutes()
	routes.Add("c1", &Route{
		Hostname:      "zebra.localhost",
		ContainerName: "zebra-1",
		Port:          "80",
	})
	routes.Add("c2", &Route{
		Hostname:      "alpha.localhost",
		ContainerName: "alpha-1",
		Port:          "80",
	})
	routes.Add("c3", &Route{
		Hostname:      "middle.localhost",
		ContainerName: "middle-1",
		Port:          "3000",
	})

	got, err := GenerateCaddyfile(routes)
	if err != nil {
		t.Fatalf("GenerateCaddyfile() error = %v", err)
	}

	// Verify order: alpha < middle < zebra
	alphaIdx := strings.Index(got, "alpha.localhost")
	middleIdx := strings.Index(got, "middle.localhost")
	zebraIdx := strings.Index(got, "zebra.localhost")

	if alphaIdx == -1 || middleIdx == -1 || zebraIdx == -1 {
		t.Fatalf("missing hostnames in output:\n%s", got)
	}
	if !(alphaIdx < middleIdx && middleIdx < zebraIdx) {
		t.Errorf("routes not sorted: alpha@%d, middle@%d, zebra@%d", alphaIdx, middleIdx, zebraIdx)
	}
}

func TestGenerateCaddyfile_RejectsInvalidHostname(t *testing.T) {
	routes := NewActiveRoutes()
	routes.Add("c1", &Route{
		Hostname:      "bad{host",
		ContainerName: "container-1",
		Port:          "80",
	})
	_, err := GenerateCaddyfile(routes)
	if err == nil {
		t.Error("expected error for invalid hostname with curly brace")
	}
}

func TestGenerateCaddyfile_RejectsInvalidPort(t *testing.T) {
	routes := NewActiveRoutes()
	routes.Add("c1", &Route{
		Hostname:      "app.localhost",
		ContainerName: "container-1",
		Port:          "abc",
	})
	_, err := GenerateCaddyfile(routes)
	if err == nil {
		t.Error("expected error for non-numeric port")
	}
}

func TestGenerateCaddyfile_RejectsInvalidContainerName(t *testing.T) {
	routes := NewActiveRoutes()
	routes.Add("c1", &Route{
		Hostname:      "app.localhost",
		ContainerName: "bad container",
		Port:          "80",
	})
	_, err := GenerateCaddyfile(routes)
	if err == nil {
		t.Error("expected error for container name with space")
	}
}

func TestGenerateCaddyfile_DuplicateHostnames_Combined(t *testing.T) {
	routes := NewActiveRoutes()
	routes.Add("c1", &Route{
		Hostname:      "worker.localhost",
		ContainerName: "worker-1",
		Port:          "8000",
	})
	routes.Add("c2", &Route{
		Hostname:      "worker.localhost",
		ContainerName: "worker-2",
		Port:          "8000",
	})
	routes.Add("c3", &Route{
		Hostname:      "worker.localhost",
		ContainerName: "worker-3",
		Port:          "8000",
	})

	got, err := GenerateCaddyfile(routes)
	if err != nil {
		t.Fatalf("GenerateCaddyfile() error = %v", err)
	}

	// Should have exactly one site block for worker.localhost
	if count := strings.Count(got, "worker.localhost {"); count != 1 {
		t.Errorf("expected 1 site block for worker.localhost, got %d\n%s", count, got)
	}

	// The reverse_proxy line should contain all three upstreams
	if !strings.Contains(got, "worker-1:8000") {
		t.Error("expected worker-1:8000 in reverse_proxy")
	}
	if !strings.Contains(got, "worker-2:8000") {
		t.Error("expected worker-2:8000 in reverse_proxy")
	}
	if !strings.Contains(got, "worker-3:8000") {
		t.Error("expected worker-3:8000 in reverse_proxy")
	}

	// Should have exactly one reverse_proxy directive
	if count := strings.Count(got, "reverse_proxy"); count != 1 {
		t.Errorf("expected 1 reverse_proxy directive, got %d\n%s", count, got)
	}
}

func TestGenerateCaddyfile_DuplicateHostnames_RejectsInvalidUpstream(t *testing.T) {
	routes := NewActiveRoutes()
	routes.Add("c1", &Route{
		Hostname:      "worker.localhost",
		ContainerName: "worker-1",
		Port:          "8000",
	})
	routes.Add("c2", &Route{
		Hostname:      "worker.localhost",
		ContainerName: "bad container",
		Port:          "8000",
	})

	_, err := GenerateCaddyfile(routes)
	if err == nil {
		t.Error("expected error for invalid container name in grouped upstreams")
	}
}

func TestActiveRoutes_AddGetRemove(t *testing.T) {
	ar := NewActiveRoutes()

	// Initially empty
	if ar.Len() != 0 {
		t.Errorf("Len() = %d, want 0", ar.Len())
	}

	// Add
	route := &Route{Hostname: "app.localhost", ContainerName: "web-1", Port: "80"}
	ar.Add("c1", route)
	if ar.Len() != 1 {
		t.Errorf("Len() = %d, want 1", ar.Len())
	}

	// Get existing
	got, ok := ar.Get("c1")
	if !ok {
		t.Fatal("Get(c1) returned not found")
	}
	if got.Hostname != "app.localhost" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "app.localhost")
	}

	// Get non-existing
	_, ok = ar.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}

	// Remove
	ar.Remove("c1")
	if ar.Len() != 0 {
		t.Errorf("Len() after Remove = %d, want 0", ar.Len())
	}

	// Remove non-existing (should not panic)
	ar.Remove("nonexistent")
}

func TestActiveRoutes_All_Sorted(t *testing.T) {
	ar := NewActiveRoutes()
	ar.Add("c1", &Route{Hostname: "zebra.localhost"})
	ar.Add("c2", &Route{Hostname: "alpha.localhost"})
	ar.Add("c3", &Route{Hostname: "middle.localhost"})

	all := ar.All()
	if len(all) != 3 {
		t.Fatalf("All() returned %d routes, want 3", len(all))
	}
	if all[0].Hostname != "alpha.localhost" {
		t.Errorf("All()[0] = %q, want alpha.localhost", all[0].Hostname)
	}
	if all[1].Hostname != "middle.localhost" {
		t.Errorf("All()[1] = %q, want middle.localhost", all[1].Hostname)
	}
	if all[2].Hostname != "zebra.localhost" {
		t.Errorf("All()[2] = %q, want zebra.localhost", all[2].Hostname)
	}
}

func TestActiveRoutes_Concurrency(t *testing.T) {
	ar := NewActiveRoutes()
	const n = 100
	var wg sync.WaitGroup

	// Concurrent adds
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := strings.Repeat("c", idx+1)
			ar.Add(id, &Route{
				Hostname:      "host.localhost",
				ContainerName: "container-1",
				Port:          "80",
			})
		}(i)
	}
	wg.Wait()

	if ar.Len() != n {
		t.Errorf("Len() = %d, want %d after concurrent adds", ar.Len(), n)
	}

	// Concurrent reads + removes
	wg = sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			ar.All()
		}(i)
		go func(idx int) {
			defer wg.Done()
			id := strings.Repeat("c", idx+1)
			ar.Remove(id)
		}(i)
	}
	wg.Wait()

	if ar.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after concurrent removes", ar.Len())
	}
}
