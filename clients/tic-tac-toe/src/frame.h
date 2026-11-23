/* RAMServer message framing (docs/framing.md): a 4-byte big-endian length prefix, then that many payload
 * bytes. A zero-length frame is a heartbeat.
 *
 * The decoder is fed whatever bytes a read returned, in any split, and hands back complete frames. It
 * never treats a read as a frame boundary. */
#ifndef TTT_FRAME_H
#define TTT_FRAME_H

#include <stddef.h>
#include <stdint.h>

#define FRAME_PREFIX_LEN 4u
#define FRAME_MAX_PAYLOAD 65536u /* docs/framing.md: maximum payload size */

/* Writes the frame for payload into out, which must hold FRAME_PREFIX_LEN + len bytes. Returns the
 * frame's length, or 0 if len exceeds FRAME_MAX_PAYLOAD. len 0 encodes a heartbeat. */
size_t frame_encode(const uint8_t *payload, size_t len, uint8_t *out);

typedef enum {
    FRAME_NEED_MORE = 0, /* no complete frame buffered yet */
    FRAME_OK = 1,        /* *payload / *len hold one frame (len 0: a heartbeat) */
    FRAME_TOO_LARGE = -1 /* the stream declared a payload over the maximum: it cannot continue */
} frame_status;

typedef struct {
    uint8_t buf[FRAME_PREFIX_LEN + FRAME_MAX_PAYLOAD]; /* at most one partial frame is kept */
    size_t len;                                        /* bytes in buf */
    size_t consumed;                                   /* bytes of buf already returned as frames */
    int broken;                                        /* set once FRAME_TOO_LARGE was returned */
} frame_decoder;

void frame_decoder_init(frame_decoder *d);

/* Appends bytes read from the connection. Returns how many were taken: fewer than len when the buffer
 * is full, in which case call frame_next until it returns FRAME_NEED_MORE and feed the rest. */
size_t frame_decoder_feed(frame_decoder *d, const uint8_t *data, size_t len);

/* Returns the next complete frame. *payload points into the decoder and stays valid until the next
 * call to frame_decoder_feed or frame_next. */
frame_status frame_next(frame_decoder *d, const uint8_t **payload, size_t *len);

#endif
