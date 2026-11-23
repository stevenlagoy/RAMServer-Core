#include "frame.h"

#include <string.h>

size_t frame_encode(const uint8_t *payload, size_t len, uint8_t *out) {
    if (len > FRAME_MAX_PAYLOAD) return 0;
    out[0] = (uint8_t)(len >> 24);
    out[1] = (uint8_t)(len >> 16);
    out[2] = (uint8_t)(len >> 8);
    out[3] = (uint8_t)len;
    if (len > 0) memcpy(out + FRAME_PREFIX_LEN, payload, len);
    return FRAME_PREFIX_LEN + len;
}

void frame_decoder_init(frame_decoder *d) {
    d->len = 0;
    d->consumed = 0;
    d->broken = 0;
}

/* Moves unconsumed bytes to the front so the buffer has room again */
static void compact(frame_decoder *d) {
    if (d->consumed == 0) return;
    memmove(d->buf, d->buf + d->consumed, d->len - d->consumed);
    d->len -= d->consumed;
    d->consumed = 0;
}

size_t frame_decoder_feed(frame_decoder *d, const uint8_t *data, size_t len) {
    if (d->broken) return len; /* the stream is dead; swallow it */
    compact(d);
    size_t room = sizeof d->buf - d->len;
    size_t n = len < room ? len : room;
    memcpy(d->buf + d->len, data, n);
    d->len += n;
    return n;
}

frame_status frame_next(frame_decoder *d, const uint8_t **payload, size_t *len) {
    if (d->broken) return FRAME_TOO_LARGE;
    size_t avail = d->len - d->consumed;
    if (avail < FRAME_PREFIX_LEN) return FRAME_NEED_MORE;
    const uint8_t *p = d->buf + d->consumed;
    uint32_t n = ((uint32_t)p[0] << 24) | ((uint32_t)p[1] << 16) | ((uint32_t)p[2] << 8) | (uint32_t)p[3];
    if (n > FRAME_MAX_PAYLOAD) { /* reject before reading the payload */
        d->broken = 1;
        return FRAME_TOO_LARGE;
    }
    if (avail < FRAME_PREFIX_LEN + n) return FRAME_NEED_MORE;
    *payload = p + FRAME_PREFIX_LEN;
    *len = n;
    d->consumed += FRAME_PREFIX_LEN + n;
    return FRAME_OK;
}
