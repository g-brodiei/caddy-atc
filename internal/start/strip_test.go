package start

import (
	"strings"
	"testing"
)

func TestStripPorts_Basic(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - "80:80"
    volumes:
      - ./html:/usr/share/nginx/html
  db:
    image: postgres
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: mydb
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected no ports: in output, got:\n%s", output)
	}
	if !strings.Contains(output, "image: nginx") {
		t.Error("expected 'image: nginx' preserved")
	}
	if !strings.Contains(output, "volumes:") {
		t.Error("expected 'volumes:' preserved")
	}
	if !strings.Contains(output, "POSTGRES_DB: mydb") {
		t.Error("expected environment preserved")
	}
}

func TestStripPorts_PreservesVariableReferences(t *testing.T) {
	input := `services:
  backend:
    build: ./backend
    ports:
      - "8000:8000"
    environment:
      - FINLAB_API_TOKEN=${FINLAB_API_TOKEN}
      - HOST_UID=${HOST_UID}
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected no ports: in output, got:\n%s", output)
	}
	if !strings.Contains(output, "${FINLAB_API_TOKEN}") {
		t.Error("expected ${FINLAB_API_TOKEN} preserved")
	}
	if !strings.Contains(output, "${HOST_UID}") {
		t.Error("expected ${HOST_UID} preserved")
	}
}

func TestStripPorts_PreservesExpose(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - "80:80"
    expose:
      - "80"
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected no ports: in output")
	}
	if !strings.Contains(output, "expose:") {
		t.Error("expected expose: preserved")
	}
}

func TestStripPorts_KeepPorts(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - "80:80"
  db:
    image: postgres
    ports:
      - "5432:5432"
  redis:
    image: redis
    ports:
      - "6379:6379"
`
	got, err := StripPorts([]byte(input), []string{"db", "redis"})
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if !strings.Contains(output, "5432:5432") {
		t.Error("expected db ports preserved with --keep-ports")
	}
	if !strings.Contains(output, "6379:6379") {
		t.Error("expected redis ports preserved with --keep-ports")
	}
	if strings.Contains(output, "80:80") {
		t.Error("expected web ports stripped")
	}
}

func TestStripPorts_NoServices(t *testing.T) {
	input := `volumes:
  data:
    driver: local
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if !strings.Contains(output, "volumes:") {
		t.Error("expected volumes preserved")
	}
}

func TestStripPorts_LongFormPorts(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - target: 80
        published: 8080
        protocol: tcp
    volumes:
      - ./html:/usr/share/nginx/html
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected long-form ports stripped, got:\n%s", output)
	}
	if !strings.Contains(output, "volumes:") {
		t.Error("expected volumes preserved")
	}
}

func TestStripPorts_PreservesProfiles(t *testing.T) {
	input := `services:
  caddy-dev:
    image: caddy:2-alpine
    profiles: [dev]
    ports:
      - "3000:3000"
  backend:
    build: ./backend
    ports:
      - "8000:8000"
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected ports stripped")
	}
	if !strings.Contains(output, "profiles:") {
		t.Error("expected profiles preserved")
	}
}

func TestStripPorts_ServiceWithNoPorts(t *testing.T) {
	input := `services:
  worker:
    build: ./backend
    command: python -m worker
    environment:
      - REDIS_URL=redis://redis:6379/0
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if !strings.Contains(output, "command: python -m worker") {
		t.Error("expected service without ports to pass through unchanged")
	}
}

func TestStripPorts_PreservesNetworksAndVolumes(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - "80:80"
    networks:
      - app-network

networks:
  app-network:
    driver: bridge

volumes:
  caddy_data:
  caddy_config:
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if !strings.Contains(output, "networks:") {
		t.Error("expected networks preserved")
	}
	if !strings.Contains(output, "app-network") {
		t.Error("expected app-network preserved")
	}
	if !strings.Contains(output, "caddy_data") {
		t.Error("expected volumes preserved")
	}
}
