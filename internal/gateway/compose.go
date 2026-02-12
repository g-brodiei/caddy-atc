package gateway

import (
	_ "embed"
)

//go:embed docker-compose.yml
var ComposeFile []byte
