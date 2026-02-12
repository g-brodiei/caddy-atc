BINARY_NAME=caddy-atc
BUILD_DIR=./build
INSTALL_DIR=$(HOME)/go/bin

.PHONY: build install clean

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/caddy-atc

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)

clean:
	rm -rf $(BUILD_DIR)
