# The tic-tac-toe client for RAMServer.
#
#   make            build build/<os>/ttt (and a ./ttt link to it)
#   make test       the unit tests (no server or display needed)
#   make integration SERVER=127.0.0.1:8080
#                   real clients play complete matches against a running RAMServer with tic-tac-toe
#   make memcheck   run the unit tests under valgrind
#   make clean
#
# Needs: a C11 compiler, buf, protoc with protoc-c, protobuf-c, libwebsockets (with OpenSSL), and cmocka
# for the tests. See README.md for the packages.

CC      ?= cc
PKGS    := libprotobuf-c libwebsockets
CFLAGS  ?= -O2 -g
CFLAGS  += -std=gnu11 -Wall -Wextra -Wshadow -Wformat=2 -Wno-format-nonliteral -Wno-format-truncation
CPPFLAGS += -Isrc -I$(GEN) -MMD -MP
LDLIBS  := $(shell pkg-config --libs $(PKGS)) -lpthread
TEST_LDLIBS := $(shell pkg-config --libs libprotobuf-c cmocka)
CFLAGS  += $(shell pkg-config --cflags $(PKGS) cmocka)

# One build directory per OS, so building on WSL and in the Docker container (which share this
# directory) never mixes binaries linked against different library versions
OS_TAG  := $(shell . /etc/os-release 2>/dev/null && echo "$$ID$$VERSION_ID" || uname -s)
BUILD   ?= build/$(OS_TAG)
GEN     := $(BUILD)/gen
# The protocol schema is fetched from RAMServer-Core's main branch, like every client branch's buf.gen.yaml
PROTO_SOURCE ?= https://github.com/stevenlagoy/RAMServer-Core.git\#branch=main,subdir=proto
PROTO_ROOT := $(BUILD)/proto
PROTO   := $(PROTO_ROOT)/ramserver/v1/ramserver.proto
PB_C    := $(GEN)/ramserver/v1/ramserver.pb-c.c
PB_H    := $(GEN)/ramserver/v1/ramserver.pb-c.h
PB_O    := $(BUILD)/ramserver.pb-c.o

# Protocol, framing and game state: no networking or terminal code, so the tests use only these
CORE    := frame view proto client addr bot
# Networking, the event loop and the user interfaces
APP     := transport app tui plain main
TESTS   := test_frame test_view test_proto test_client test_addr test_bot

CORE_O  := $(CORE:%=$(BUILD)/%.o) $(PB_O)
APP_O   := $(APP:%=$(BUILD)/%.o)
TEST_BIN := $(TESTS:%=$(BUILD)/%)

.PHONY: all ttt test unit integration memcheck proto clean
.SECONDARY:
all: ttt

ttt: $(BUILD)/ttt
	ln -sf $(BUILD)/ttt ttt

$(BUILD)/ttt: $(CORE_O) $(APP_O)
	$(CC) $(CFLAGS) -o $@ $^ $(LDLIBS)

# The schema, downloaded once per build directory; `make proto` fetches the latest again
$(PROTO):
	@mkdir -p $(PROTO_ROOT)
	buf export '$(PROTO_SOURCE)' -o $(PROTO_ROOT)

proto:
	rm -rf $(PROTO_ROOT) $(GEN)
	$(MAKE) $(PB_H)

# Generated protobuf code (not committed; see the repository's .gitignore)
$(PB_C) $(PB_H) &: $(PROTO)
	@mkdir -p $(GEN)
	protoc -I$(PROTO_ROOT) --c_out=$(GEN) $(PROTO)

$(PB_O): $(PB_C)
	$(CC) $(CPPFLAGS) $(filter-out -Wshadow,$(CFLAGS)) -w -c -o $@ $<

$(BUILD)/%.o: src/%.c $(PB_H)
	@mkdir -p $(BUILD)
	$(CC) $(CPPFLAGS) $(CFLAGS) -c -o $@ $<

$(BUILD)/test_%.o: tests/test_%.c $(PB_H)
	@mkdir -p $(BUILD)
	$(CC) $(CPPFLAGS) -Itests $(CFLAGS) -c -o $@ $<

$(BUILD)/test_helpers.o: tests/helpers.c $(PB_H)
	@mkdir -p $(BUILD)
	$(CC) $(CPPFLAGS) -Itests $(CFLAGS) -c -o $@ $<

$(BUILD)/test_%: $(BUILD)/test_%.o $(BUILD)/test_helpers.o $(CORE_O)
	$(CC) $(CFLAGS) -o $@ $^ $(TEST_LDLIBS)

test: unit
	@echo "All client unit tests passed."

unit: $(TEST_BIN)
	@set -e; for t in $(TEST_BIN); do echo "== $$t"; ./$$t; done

# Needs a running server with tic-tac-toe, given as SERVER (any address ttt accepts)
integration: $(BUILD)/ttt
	@if [ -z "$(SERVER)" ]; then echo "usage: make integration SERVER=127.0.0.1:8080 (a running RAMServer with tic-tac-toe)"; exit 2; fi
	tests/integration.sh $(BUILD) $(SERVER)

memcheck: $(TEST_BIN)
	@set -e; for t in $(TEST_BIN); do echo "== valgrind $$t"; \
		valgrind -q --error-exitcode=1 --leak-check=full --errors-for-leak-kinds=definite ./$$t; done

clean:
	rm -rf build ttt

-include $(wildcard $(BUILD)/*.d)
