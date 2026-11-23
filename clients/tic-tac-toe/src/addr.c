#include "addr.h"

#include <ctype.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static bool fail(char *err, size_t n, const char *msg, const char *spec) {
    if (err && n) snprintf(err, n, "%s: \"%s\"", msg, spec);
    return false;
}

/* Parses HOST[:PORT] from s[0..len) into out. default_port < 0 means the port is required. */
static bool host_port(const char *s, size_t len, int default_port, server_addr *out, char *err, size_t n,
                      const char *spec) {
    const char *end = s + len;
    const char *host = s, *host_end, *colon = NULL;
    if (len == 0) return fail(err, n, "missing host", spec);
    if (*s == '[') { /* [IPv6]:port */
        const char *close = memchr(s, ']', len);
        if (!close) return fail(err, n, "unterminated [ in address", spec);
        host = s + 1;
        host_end = close;
        if (close + 1 < end) {
            if (close[1] != ':') return fail(err, n, "expected :PORT after ]", spec);
            colon = close + 1;
        }
    } else {
        colon = NULL;
        for (const char *p = s; p < end; p++)
            if (*p == ':') colon = p;
        host_end = colon ? colon : end;
    }
    size_t hl = (size_t)(host_end - host);
    if (hl == 0 || hl >= sizeof out->host) return fail(err, n, "bad host", spec);
    memcpy(out->host, host, hl);
    out->host[hl] = '\0';
    if (colon) {
        const char *p = colon + 1;
        if (p == end) return fail(err, n, "missing port after :", spec);
        long port = 0;
        for (; p < end; p++) {
            if (!isdigit((unsigned char)*p)) return fail(err, n, "bad port", spec);
            port = port * 10 + (*p - '0');
            if (port > 65535) return fail(err, n, "port out of range", spec);
        }
        if (port == 0) return fail(err, n, "port out of range", spec);
        out->port = (int)port;
    } else {
        if (default_port < 0) return fail(err, n, "missing :PORT", spec);
        out->port = default_port;
    }
    return true;
}

static void make_label(server_addr *a) {
    bool v6 = strchr(a->host, ':') != NULL;
    const char *lb = v6 ? "[" : "", *rb = v6 ? "]" : "";
    switch (a->kind) {
    case ADDR_TCP: snprintf(a->label, sizeof a->label, "tcp://%s%s%s:%d", lb, a->host, rb, a->port); break;
    case ADDR_WS:
        if (a->port == 80) snprintf(a->label, sizeof a->label, "ws://%s%s%s%s", lb, a->host, rb, a->path);
        else snprintf(a->label, sizeof a->label, "ws://%s%s%s:%d%s", lb, a->host, rb, a->port, a->path);
        break;
    case ADDR_WSS:
        if (a->port == 443) snprintf(a->label, sizeof a->label, "wss://%s%s%s%s", lb, a->host, rb, a->path);
        else snprintf(a->label, sizeof a->label, "wss://%s%s%s:%d%s", lb, a->host, rb, a->port, a->path);
        break;
    }
}

bool addr_parse(const char *spec, server_addr *out, char *err, size_t errlen) {
    server_addr a;
    memset(&a, 0, sizeof a);
    if (spec == NULL || *spec == '\0') return fail(err, errlen, "empty server address", "");
    if (strcmp(spec, "local") == 0) spec = ADDR_LOCAL_TCP;
    else if (strcmp(spec, "local-ws") == 0) spec = ADDR_LOCAL_WS;
    else if (strcmp(spec, "hosted") == 0) spec = ADDR_HOSTED_URL;

    const char *rest;
    int default_port;
    if (strncmp(spec, "wss://", 6) == 0) {
        a.kind = ADDR_WSS, rest = spec + 6, default_port = 443;
    } else if (strncmp(spec, "ws://", 5) == 0) {
        a.kind = ADDR_WS, rest = spec + 5, default_port = 80;
    } else if (strncmp(spec, "tcp://", 6) == 0) {
        a.kind = ADDR_TCP, rest = spec + 6, default_port = -1;
    } else if (strstr(spec, "://") != NULL) {
        return fail(err, errlen, "unsupported scheme (use tcp://, ws:// or wss://)", spec);
    } else {
        a.kind = ADDR_TCP, rest = spec, default_port = -1;
    }

    const char *slash = strchr(rest, '/');
    size_t hp_len = slash ? (size_t)(slash - rest) : strlen(rest);
    if (!host_port(rest, hp_len, default_port, &a, err, errlen, spec)) return false;
    if (a.kind == ADDR_TCP) {
        if (slash && slash[1] != '\0') return fail(err, errlen, "a TCP address has no path", spec);
    } else {
        const char *path = (slash && slash[1] != '\0') ? slash : "/connect";
        if (strlen(path) >= sizeof a.path) return fail(err, errlen, "path too long", spec);
        strcpy(a.path, path);
    }
    make_label(&a);
    *out = a;
    return true;
}
