BINARY_NAME=caddy-atc
BUILD_DIR=./build
INSTALL_DIR=$(HOME)/go/bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS=-s -w -X main.version=$(VERSION)

.PHONY: build install install-completions clean check lint vulncheck

build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/caddy-atc

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)

install-completions: build
	@SHELL_NAME=$$(basename "$$SHELL"); \
	case "$$SHELL_NAME" in \
		zsh) \
			if [ "$$(uname -s)" = "Darwin" ] && command -v brew >/dev/null 2>&1; then \
				COMP_DIR="$$(brew --prefix)/share/zsh/site-functions"; \
			elif [ -d "/usr/local/share/zsh/site-functions" ] && [ -w "/usr/local/share/zsh/site-functions" ]; then \
				COMP_DIR="/usr/local/share/zsh/site-functions"; \
			else \
				COMP_DIR="$(HOME)/.zsh/completions"; \
			fi; \
			mkdir -p "$$COMP_DIR"; \
			$(BUILD_DIR)/$(BINARY_NAME) completion zsh > "$$COMP_DIR/_caddy-atc"; \
			echo "Zsh completions installed to $$COMP_DIR/_caddy-atc"; \
			;; \
		bash) \
			if [ -d "/etc/bash_completion.d" ] && [ -w "/etc/bash_completion.d" ]; then \
				COMP_DIR="/etc/bash_completion.d"; \
			else \
				COMP_DIR="$(HOME)/.local/share/bash-completion/completions"; \
			fi; \
			mkdir -p "$$COMP_DIR"; \
			$(BUILD_DIR)/$(BINARY_NAME) completion bash > "$$COMP_DIR/caddy-atc"; \
			echo "Bash completions installed to $$COMP_DIR/caddy-atc"; \
			;; \
		fish) \
			COMP_DIR="$(HOME)/.config/fish/completions"; \
			mkdir -p "$$COMP_DIR"; \
			$(BUILD_DIR)/$(BINARY_NAME) completion fish > "$$COMP_DIR/caddy-atc.fish"; \
			echo "Fish completions installed to $$COMP_DIR/caddy-atc.fish"; \
			;; \
		*) \
			echo "Unsupported shell: $$SHELL_NAME. Run 'caddy-atc completion --help' for manual setup."; \
			exit 1; \
			;; \
	esac
	@echo "Restart your shell or open a new terminal to activate."

clean:
	rm -rf $(BUILD_DIR)

lint:
	go vet ./...

vulncheck:
	@which govulncheck > /dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

check: lint vulncheck
	go test ./... -count=1
	go build -o /dev/null ./cmd/caddy-atc
