# https://github.com/aperturerobotics/template
PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
SHELL:=bash
MAKEFLAGS += --no-print-directory

GO_VENDOR_DIR := ./vendor
COMMON_DIR := $(GO_VENDOR_DIR)/github.com/aperturerobotics/common

COMMON_MAKEFILE := $(COMMON_DIR)/Makefile
TOOLS_DIR := .tools
TOOLS_MAKEFILE := .tools/Makefile

export GO111MODULE=on
undefine GOARCH
undefine GOOS

.PHONY: $(MAKECMDGOALS)

all:

$(COMMON_MAKEFILE): vendor
	@if [ ! -f $(COMMON_MAKEFILE) ]; then \
		echo "Please add github.com/aperturerobotics/common to your go.mod."; \
		exit 1; \
	fi

$(TOOLS_MAKEFILE): $(COMMON_MAKEFILE)
	@$(MAKE) -C $(COMMON_DIR) TOOLS_DIR="$(TOOLS_DIR)" PROJECT_DIR="$(PROJECT_DIR)" tools

$(MAKECMDGOALS): $(TOOLS_MAKEFILE)
	@$(MAKE) -C $(TOOLS_DIR) TOOLS_DIR="$(TOOLS_DIR)" PROJECT_DIR="$(PROJECT_DIR)" $@

%: $(TOOLS_MAKEFILE)
	@$(MAKE) -C $(TOOLS_DIR) TOOLS_DIR="$(TOOLS_DIR)" PROJECT_DIR="$(PROJECT_DIR)" $@

vendor:
	go mod vendor
