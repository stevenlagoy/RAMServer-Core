/* Server addresses: where to connect, and over which transport. */
#ifndef TTT_ADDR_H
#define TTT_ADDR_H

#include <stdbool.h>
#include <stddef.h>

typedef enum { ADDR_TCP, ADDR_WS, ADDR_WSS } addr_kind;

typedef struct {
    addr_kind kind;
    char host[256];
    int port;
    char path[256];  /* WebSocket path, e.g. "/connect" */
    char label[320]; /* human-readable, e.g. "wss://host/connect" */
} server_addr;

#define ADDR_HOSTED_URL "wss://ramserver-core-80752473398.us-east5.run.app/connect"
#define ADDR_LOCAL_TCP "127.0.0.1:8080"
#define ADDR_LOCAL_WS "ws://127.0.0.1:8081/connect"

/* Parses a server spec:
 *   local            plain TCP to 127.0.0.1:8080 (the local server's default)
 *   local-ws         ws://127.0.0.1:8081/connect
 *   hosted           the hosted server, wss://ramserver-core-80752473398.us-east5.run.app/connect
 *   HOST:PORT        plain TCP, also tcp://HOST:PORT ([v6addr]:PORT for IPv6)
 *   ws://HOST[:PORT][/PATH], wss://HOST[:PORT][/PATH]   PATH defaults to /connect
 * Returns false and writes a message to err for anything else. */
bool addr_parse(const char *spec, server_addr *out, char *err, size_t errlen);

#endif
