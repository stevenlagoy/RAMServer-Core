/* One connection to a server, over plain TCP or WebSocket (ws:// or wss://), behind the same
 * "send frame / receive bytes" interface.
 *
 * Received bytes are read from a file descriptor the caller polls (transport_fd), so one poll() loop
 * can wait on the connection and the keyboard together. For WebSocket, libwebsockets runs on its own
 * thread and passes the bytes of each binary message through a socketpair. Either way the bytes form
 * the framed stream of docs/framing.md, which the caller decodes. */
#ifndef TTT_TRANSPORT_H
#define TTT_TRANSPORT_H

#include <stddef.h>
#include <stdint.h>
#include <sys/types.h>

#include "addr.h"

typedef enum { TRANSPORT_CONNECTING, TRANSPORT_OPEN, TRANSPORT_CLOSED } transport_state;

typedef struct transport transport;

/* Starts connecting. Plain TCP connects before returning (with a timeout); WebSocket connects in the
 * background, so poll transport_state. Returns NULL with a message in err if it can't even start. */
transport *transport_open(const server_addr *addr, char *err, size_t errlen);

/* The descriptor to poll for incoming bytes */
int transport_fd(const transport *t);

/* The connection's state; for TRANSPORT_CLOSED, why, in reason. */
transport_state transport_get_state(transport *t, char *reason, size_t reasonlen);

/* Reads received bytes. Returns the count, 0 once the connection has closed and everything was read,
 * or -1 on error (errno set; EAGAIN if nothing is available). */
ssize_t transport_read(transport *t, uint8_t *buf, size_t cap);

/* Sends one complete frame (prefix and payload). Over WebSocket, as one binary message. Returns 0, or
 * -1 if the connection can't send. */
int transport_send_frame(transport *t, const uint8_t *frame, size_t len);

/* Closes the connection (over WebSocket, with a close frame first) and frees t. */
void transport_close(transport *t);

#endif
