/* Framing (docs/framing.md): encoding, heartbeats, and decoding from reads split at any byte. */
#include <stdlib.h>
#include <string.h>

#include "frame.h"
#include "helpers.h"

static void test_encode_hello_example(void **s) {
    (void)s;
    uint8_t out[16];
    size_t n = frame_encode((const uint8_t *)"hello", 5, out);
    const uint8_t want[] = {0, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'};
    assert_int_equal(n, sizeof want);
    assert_memory_equal(out, want, sizeof want);
}

static void test_encode_heartbeat(void **s) {
    (void)s;
    uint8_t out[4] = {9, 9, 9, 9};
    assert_int_equal(frame_encode(NULL, 0, out), 4);
    const uint8_t want[] = {0, 0, 0, 0};
    assert_memory_equal(out, want, 4);
}

static void test_encode_limits(void **s) {
    (void)s;
    uint8_t *big = calloc(1, FRAME_PREFIX_LEN + FRAME_MAX_PAYLOAD + 1);
    assert_int_equal(frame_encode(big, FRAME_MAX_PAYLOAD, big), FRAME_PREFIX_LEN + FRAME_MAX_PAYLOAD);
    assert_int_equal(big[0], 0);
    assert_int_equal(big[1], 1); /* 65536 = 0x00010000 */
    assert_int_equal(big[2], 0);
    assert_int_equal(big[3], 0);
    assert_int_equal(frame_encode(big, FRAME_MAX_PAYLOAD + 1, big), 0);
    free(big);
}

/* Builds a stream of frames for the given payloads (NULL payload = heartbeat) */
static size_t stream(uint8_t *out, const char *const *payloads, int n) {
    size_t len = 0;
    for (int i = 0; i < n; i++) {
        const char *p = payloads[i];
        len += frame_encode((const uint8_t *)p, p ? strlen(p) : 0, out + len);
    }
    return len;
}

static const char *const PAYLOADS[] = {"first", NULL, "second message", "x", NULL, "the last one"};
#define NPAY 6

/* Feeds the stream in chunks of chunk bytes and checks every frame comes out once, in order */
static void check_chunked(size_t chunk) {
    uint8_t buf[256];
    size_t len = stream(buf, PAYLOADS, NPAY);
    frame_decoder *d = malloc(sizeof *d);
    frame_decoder_init(d);
    int got = 0;
    for (size_t off = 0; off < len; off += chunk) {
        size_t n = len - off < chunk ? len - off : chunk;
        assert_int_equal(frame_decoder_feed(d, buf + off, n), n);
        const uint8_t *p;
        size_t plen;
        frame_status st;
        while ((st = frame_next(d, &p, &plen)) == FRAME_OK) {
            assert_true(got < NPAY);
            const char *want = PAYLOADS[got];
            assert_int_equal(plen, want ? strlen(want) : 0);
            if (want) assert_memory_equal(p, want, plen);
            got++;
        }
        assert_int_equal(st, FRAME_NEED_MORE);
    }
    assert_int_equal(got, NPAY);
    free(d);
}

static void test_one_byte_at_a_time(void **s) { (void)s; check_chunked(1); }
static void test_split_prefixes(void **s) { (void)s; for (size_t c = 2; c <= 7; c++) check_chunked(c); }
static void test_everything_in_one_read(void **s) { (void)s; check_chunked(1000); }

static void test_partial_frame_waits(void **s) {
    (void)s;
    uint8_t buf[32];
    size_t len = frame_encode((const uint8_t *)"abcdef", 6, buf);
    frame_decoder d;
    frame_decoder_init(&d);
    const uint8_t *p;
    size_t n;
    frame_decoder_feed(&d, buf, 3); /* part of the prefix */
    assert_int_equal(frame_next(&d, &p, &n), FRAME_NEED_MORE);
    frame_decoder_feed(&d, buf + 3, 4); /* prefix done, part of the payload */
    assert_int_equal(frame_next(&d, &p, &n), FRAME_NEED_MORE);
    frame_decoder_feed(&d, buf + 7, len - 7);
    assert_int_equal(frame_next(&d, &p, &n), FRAME_OK);
    assert_int_equal(n, 6);
    assert_memory_equal(p, "abcdef", 6);
    assert_int_equal(frame_next(&d, &p, &n), FRAME_NEED_MORE);
}

static void test_combined_frame_and_partial(void **s) {
    (void)s;
    uint8_t buf[64];
    size_t a = frame_encode((const uint8_t *)"one", 3, buf);
    size_t b = frame_encode((const uint8_t *)"two!", 4, buf + a);
    frame_decoder d;
    frame_decoder_init(&d);
    const uint8_t *p;
    size_t n;
    frame_decoder_feed(&d, buf, a + 5); /* a whole frame plus 5 bytes of the next */
    assert_int_equal(frame_next(&d, &p, &n), FRAME_OK);
    assert_memory_equal(p, "one", 3);
    assert_int_equal(frame_next(&d, &p, &n), FRAME_NEED_MORE);
    frame_decoder_feed(&d, buf + a + 5, b - 5);
    assert_int_equal(frame_next(&d, &p, &n), FRAME_OK);
    assert_int_equal(n, 4);
    assert_memory_equal(p, "two!", 4);
}

static void test_max_size_frame_accepted(void **s) {
    (void)s;
    uint8_t *buf = malloc(FRAME_PREFIX_LEN + FRAME_MAX_PAYLOAD);
    memset(buf + FRAME_PREFIX_LEN, 'z', FRAME_MAX_PAYLOAD);
    frame_encode(buf + FRAME_PREFIX_LEN, FRAME_MAX_PAYLOAD, buf);
    frame_decoder *d = malloc(sizeof *d);
    frame_decoder_init(d);
    size_t total = FRAME_PREFIX_LEN + FRAME_MAX_PAYLOAD, off = 0;
    const uint8_t *p;
    size_t n;
    while (off < total) off += frame_decoder_feed(d, buf + off, total - off > 1000 ? 1000 : total - off);
    assert_int_equal(frame_next(d, &p, &n), FRAME_OK);
    assert_int_equal(n, FRAME_MAX_PAYLOAD);
    assert_int_equal(p[FRAME_MAX_PAYLOAD - 1], 'z');
    free(d);
    free(buf);
}

static void test_oversized_length_rejected_before_payload(void **s) {
    (void)s;
    const uint8_t prefix[] = {0, 1, 0, 1}; /* 65537 */
    frame_decoder d;
    frame_decoder_init(&d);
    const uint8_t *p;
    size_t n;
    frame_decoder_feed(&d, prefix, 4);
    assert_int_equal(frame_next(&d, &p, &n), FRAME_TOO_LARGE);
    /* the stream can't recover: later bytes never become frames */
    uint8_t ok[8];
    frame_decoder_feed(&d, ok, frame_encode((const uint8_t *)"hi", 2, ok));
    assert_int_equal(frame_next(&d, &p, &n), FRAME_TOO_LARGE);
}

static void test_huge_length_rejected(void **s) {
    (void)s;
    const uint8_t prefix[] = {0xff, 0xff, 0xff, 0xff};
    frame_decoder d;
    frame_decoder_init(&d);
    const uint8_t *p;
    size_t n;
    frame_decoder_feed(&d, prefix, 4);
    assert_int_equal(frame_next(&d, &p, &n), FRAME_TOO_LARGE);
}

static void test_feed_more_than_buffer(void **s) {
    (void)s;
    /* Two max-size frames in one big read: feed takes what fits, the rest goes in after draining */
    size_t one = FRAME_PREFIX_LEN + FRAME_MAX_PAYLOAD;
    uint8_t *buf = calloc(2, one);
    frame_encode(buf + FRAME_PREFIX_LEN, FRAME_MAX_PAYLOAD, buf);
    frame_encode(buf + one + FRAME_PREFIX_LEN, FRAME_MAX_PAYLOAD, buf + one);
    frame_decoder *d = malloc(sizeof *d);
    frame_decoder_init(d);
    size_t off = 0;
    int frames = 0;
    while (off < 2 * one) {
        off += frame_decoder_feed(d, buf + off, 2 * one - off);
        const uint8_t *p;
        size_t n;
        while (frame_next(d, &p, &n) == FRAME_OK) frames++;
    }
    assert_int_equal(frames, 2);
    free(d);
    free(buf);
}

int main(void) {
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_encode_hello_example),
        cmocka_unit_test(test_encode_heartbeat),
        cmocka_unit_test(test_encode_limits),
        cmocka_unit_test(test_one_byte_at_a_time),
        cmocka_unit_test(test_split_prefixes),
        cmocka_unit_test(test_everything_in_one_read),
        cmocka_unit_test(test_partial_frame_waits),
        cmocka_unit_test(test_combined_frame_and_partial),
        cmocka_unit_test(test_max_size_frame_accepted),
        cmocka_unit_test(test_oversized_length_rejected_before_payload),
        cmocka_unit_test(test_huge_length_rejected),
        cmocka_unit_test(test_feed_more_than_buffer),
    };
    return cmocka_run_group_tests(tests, NULL, NULL);
}
