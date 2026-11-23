#define _GNU_SOURCE /* MSG_NOSIGNAL, strcasestr */
#include "transport.h"

#include <errno.h>
#include <fcntl.h>
#include <libwebsockets.h>
#include <netdb.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <poll.h>
#include <pthread.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

#define TCP_CONNECT_TIMEOUT_MS 5000
#define WS_TIMEOUT_SECS 30 /* the hosted server can take several seconds to wake up */

typedef struct ws_out {
    struct ws_out *next;
    size_t len;
    unsigned char buf[]; /* LWS_PRE bytes of headroom, then the frame */
} ws_out;

struct transport {
    addr_kind kind;
    pthread_mutex_t lock;
    transport_state state;
    char reason[256];

    /* TCP */
    int sock;

    /* WebSocket */
    struct lws_context *ctx;
    struct lws *wsi;
    pthread_t thread;
    bool thread_started;
    int pair_reader; /* main thread reads received bytes here */
    int pair_writer; /* the lws thread writes them here; -1 once closed */
    ws_out *out_head, *out_tail;
    bool closing;    /* send a close frame once the queue is empty */
    volatile bool stop;
    char proxy_host[256];
    char host_header[300];
    char ca_file[512];
};

static void set_closed(transport *t, const char *reason) {
    pthread_mutex_lock(&t->lock);
    if (t->state != TRANSPORT_CLOSED) {
        t->state = TRANSPORT_CLOSED;
        snprintf(t->reason, sizeof t->reason, "%s", reason && *reason ? reason : "connection closed");
    }
    pthread_mutex_unlock(&t->lock);
}

static transport *alloc_transport(addr_kind kind) {
    transport *t = calloc(1, sizeof *t);
    if (!t) return NULL;
    t->kind = kind;
    pthread_mutex_init(&t->lock, NULL);
    t->sock = t->pair_reader = t->pair_writer = -1;
    return t;
}

static void free_transport(transport *t) {
    for (ws_out *o = t->out_head; o;) {
        ws_out *next = o->next;
        free(o);
        o = next;
    }
    if (t->sock >= 0) close(t->sock);
    if (t->pair_reader >= 0) close(t->pair_reader);
    if (t->pair_writer >= 0) close(t->pair_writer);
    pthread_mutex_destroy(&t->lock);
    free(t);
}

/* ------------------------------------------------------------------------------------------------ */
/* Plain TCP                                                                                         */

static int tcp_connect(const server_addr *a, char *err, size_t errlen) {
    char port[16];
    snprintf(port, sizeof port, "%d", a->port);
    struct addrinfo hints = {0}, *res = NULL;
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    int rc = getaddrinfo(a->host, port, &hints, &res);
    if (rc != 0) {
        snprintf(err, errlen, "can't resolve %s: %s", a->host, gai_strerror(rc));
        return -1;
    }
    int fd = -1;
    int last_errno = 0;
    for (struct addrinfo *ai = res; ai; ai = ai->ai_next) {
        fd = socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
        if (fd < 0) { last_errno = errno; continue; }
        int flags = fcntl(fd, F_GETFL, 0);
        fcntl(fd, F_SETFL, flags | O_NONBLOCK);
        rc = connect(fd, ai->ai_addr, ai->ai_addrlen);
        if (rc < 0 && errno == EINPROGRESS) {
            struct pollfd p = {fd, POLLOUT, 0};
            rc = poll(&p, 1, TCP_CONNECT_TIMEOUT_MS);
            if (rc == 0) { last_errno = ETIMEDOUT; rc = -1; }
            else if (rc > 0) {
                int soerr = 0;
                socklen_t sl = sizeof soerr;
                getsockopt(fd, SOL_SOCKET, SO_ERROR, &soerr, &sl);
                if (soerr) { last_errno = soerr; rc = -1; }
                else rc = 0;
            } else last_errno = errno;
        } else if (rc < 0) last_errno = errno;
        if (rc == 0) {
            fcntl(fd, F_SETFL, flags); /* back to blocking: sends write everything */
            int one = 1;
            setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, &one, sizeof one);
            break;
        }
        close(fd);
        fd = -1;
    }
    freeaddrinfo(res);
    if (fd < 0) snprintf(err, errlen, "can't connect to %s:%d: %s", a->host, a->port, strerror(last_errno));
    return fd;
}

static int tcp_send(transport *t, const uint8_t *buf, size_t len) {
    while (len > 0) {
        ssize_t n = send(t->sock, buf, len, MSG_NOSIGNAL);
        if (n < 0) {
            if (errno == EINTR) continue;
            set_closed(t, strerror(errno));
            return -1;
        }
        buf += n;
        len -= (size_t)n;
    }
    return 0;
}

/* ------------------------------------------------------------------------------------------------ */
/* WebSocket, through libwebsockets on its own thread                                                */

static char lws_last_error[256];

static void lws_log(int level, const char *line) {
    (void)level;
    /* lws would print to stderr, under the UI. Keep the last line to explain a failure instead. */
    snprintf(lws_last_error, sizeof lws_last_error, "%s", line);
    size_t n = strlen(lws_last_error);
    while (n > 0 && (lws_last_error[n - 1] == '\n' || lws_last_error[n - 1] == '\r')) lws_last_error[--n] = '\0';
    if (getenv("TTT_LWS_LOG")) fprintf(stderr, "lws: %s\n", lws_last_error);
}

static void ws_finish(transport *t, const char *reason) {
    set_closed(t, reason);
    pthread_mutex_lock(&t->lock);
    if (t->pair_writer >= 0) { /* the reader sees EOF after the bytes already passed on */
        close(t->pair_writer);
        t->pair_writer = -1;
    }
    t->wsi = NULL;
    pthread_mutex_unlock(&t->lock);
}

static int ws_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len) {
    (void)user;
    transport *t = lws_context_user(lws_get_context(wsi));
    if (t == NULL) return 0;
    switch (reason) {
    case LWS_CALLBACK_CLIENT_ESTABLISHED:
        pthread_mutex_lock(&t->lock);
        if (t->state == TRANSPORT_CONNECTING) t->state = TRANSPORT_OPEN;
        bool pending = t->out_head != NULL;
        pthread_mutex_unlock(&t->lock);
        if (pending) lws_callback_on_writable(wsi);
        break;

    case LWS_CALLBACK_CLIENT_RECEIVE: {
        if (lws_frame_is_binary(wsi) == 0) break; /* the server only sends binary messages */
        const uint8_t *p = in;
        while (len > 0) {
            ssize_t n = write(t->pair_writer, p, len);
            if (n < 0) {
                if (errno == EINTR) continue;
                return -1; /* the reader is gone */
            }
            p += n;
            len -= (size_t)n;
        }
        break;
    }

    case LWS_CALLBACK_CLIENT_WRITEABLE: {
        pthread_mutex_lock(&t->lock);
        ws_out *o = t->out_head;
        if (o) {
            t->out_head = o->next;
            if (!t->out_head) t->out_tail = NULL;
        }
        bool close_now = !o && t->closing;
        pthread_mutex_unlock(&t->lock);
        if (close_now) {
            lws_close_reason(wsi, LWS_CLOSE_STATUS_NORMAL, NULL, 0);
            return -1; /* sends the close frame */
        }
        if (o) {
            int n = lws_write(wsi, o->buf + LWS_PRE, o->len, LWS_WRITE_BINARY);
            bool ok = n >= (int)o->len;
            free(o);
            if (!ok) return -1;
        }
        pthread_mutex_lock(&t->lock);
        bool more = t->out_head != NULL || t->closing;
        pthread_mutex_unlock(&t->lock);
        if (more) lws_callback_on_writable(wsi);
        break;
    }

    case LWS_CALLBACK_EVENT_WAIT_CANCELLED: {
        /* transport_send_frame / transport_close woke us: ask to write on our connection */
        pthread_mutex_lock(&t->lock);
        struct lws *w = t->wsi;
        bool want = w && t->state == TRANSPORT_OPEN && (t->out_head || t->closing);
        pthread_mutex_unlock(&t->lock);
        if (want) lws_callback_on_writable(w);
        break;
    }

    case LWS_CALLBACK_CLIENT_CONNECTION_ERROR: {
        char why[256];
        if (in && len == 0) len = strlen(in);
        if (in && len) snprintf(why, sizeof why, "%.*s", (int)len, (const char *)in);
        else snprintf(why, sizeof why, "%s", lws_last_error[0] ? lws_last_error : "WebSocket connection failed");
        ws_finish(t, why);
        break;
    }

    case LWS_CALLBACK_WS_PEER_INITIATED_CLOSE:
        set_closed(t, "the server closed the connection");
        break;

    case LWS_CALLBACK_CLIENT_CLOSED: ws_finish(t, "the server closed the connection"); break;

    case LWS_CALLBACK_WSI_DESTROY:
        if (wsi == t->wsi) ws_finish(t, "connection closed");
        break;

    default: break;
    }
    return 0;
}

static const struct lws_protocols ws_protocols[] = {
    {"ramserver", ws_callback, 0, 4096, 0, NULL, 0},
    {NULL, NULL, 0, 0, 0, NULL, 0},
};

static void *ws_thread(void *arg) {
    transport *t = arg;
    while (!t->stop) lws_service(t->ctx, 0);
    return NULL;
}

/* Is host covered by NO_PROXY? Handles exact names and ".suffix"/"suffix" domain entries. */
static bool no_proxy_for(const char *host) {
    const char *np = getenv("no_proxy");
    if (!np) np = getenv("NO_PROXY");
    if (!np) return false;
    char list[2048];
    snprintf(list, sizeof list, "%s", np);
    size_t hl = strlen(host);
    for (char *save = NULL, *tok = strtok_r(list, ",", &save); tok; tok = strtok_r(NULL, ",", &save)) {
        while (*tok == ' ') tok++;
        if (*tok == '*' && tok[1] == '\0') return true;
        if (*tok == '*') tok++;
        if (*tok == '.') tok++;
        size_t tl = strlen(tok);
        if (tl == 0 || tl > hl) continue;
        if (strcasecmp(host + hl - tl, tok) == 0 && (hl == tl || host[hl - tl - 1] == '.')) return true;
    }
    return false;
}

/* For wss://, honours HTTPS_PROXY (http://[user:pass@]host:port), as most native tools do */
static void ws_proxy(transport *t, const server_addr *a, struct lws_context_creation_info *info) {
    if (a->kind != ADDR_WSS || no_proxy_for(a->host)) return;
    const char *p = getenv("https_proxy");
    if (!p || !*p) p = getenv("HTTPS_PROXY");
    if (!p || !*p) return;
    if (strncmp(p, "http://", 7) == 0) p += 7;
    snprintf(t->proxy_host, sizeof t->proxy_host, "%s", p);
    char *slash = strchr(t->proxy_host, '/');
    if (slash) *slash = '\0';
    char *colon = strrchr(t->proxy_host, ':');
    unsigned port = 80;
    if (colon && !strchr(colon, ']')) {
        port = (unsigned)atoi(colon + 1);
        *colon = '\0';
    }
    info->http_proxy_address = t->proxy_host;
    info->http_proxy_port = port;
}

/* The CA bundle to verify wss:// servers with: TTT_CA_FILE, SSL_CERT_FILE, or the system bundle */
static void ws_ca_file(transport *t, struct lws_context_creation_info *info) {
    const char *candidates[] = {getenv("TTT_CA_FILE"), getenv("SSL_CERT_FILE"), "/etc/ssl/certs/ca-certificates.crt",
                                "/etc/pki/tls/certs/ca-bundle.crt", "/etc/ssl/cert.pem"};
    for (size_t i = 0; i < sizeof candidates / sizeof candidates[0]; i++) {
        if (candidates[i] && *candidates[i] && access(candidates[i], R_OK) == 0) {
            snprintf(t->ca_file, sizeof t->ca_file, "%s", candidates[i]);
            info->client_ssl_ca_filepath = t->ca_file;
            return;
        }
    }
}

static transport *ws_open(const server_addr *a, char *err, size_t errlen) {
    transport *t = alloc_transport(a->kind);
    if (!t) {
        snprintf(err, errlen, "out of memory");
        return NULL;
    }
    t->state = TRANSPORT_CONNECTING;
    int pair[2];
    if (socketpair(AF_UNIX, SOCK_STREAM, 0, pair) < 0) {
        snprintf(err, errlen, "socketpair: %s", strerror(errno));
        free_transport(t);
        return NULL;
    }
    t->pair_reader = pair[0];
    t->pair_writer = pair[1];
    fcntl(t->pair_reader, F_SETFL, fcntl(t->pair_reader, F_GETFL, 0) | O_NONBLOCK);

    lws_set_log_level(LLL_ERR | LLL_WARN, lws_log);
    lws_last_error[0] = '\0';

    struct lws_context_creation_info info;
    memset(&info, 0, sizeof info);
    info.port = CONTEXT_PORT_NO_LISTEN;
    info.protocols = ws_protocols;
    info.gid = (gid_t)-1;
    info.uid = (uid_t)-1;
    info.user = t;
    info.timeout_secs = WS_TIMEOUT_SECS;
    if (a->kind == ADDR_WSS) {
        info.options |= LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
        ws_ca_file(t, &info);
    }
    ws_proxy(t, a, &info);
    t->ctx = lws_create_context(&info);
    if (!t->ctx) {
        snprintf(err, errlen, "can't start WebSocket support%s%s", lws_last_error[0] ? ": " : "", lws_last_error);
        free_transport(t);
        return NULL;
    }

    bool default_port = (a->kind == ADDR_WSS && a->port == 443) || (a->kind == ADDR_WS && a->port == 80);
    if (default_port || a->kind == ADDR_WSS) snprintf(t->host_header, sizeof t->host_header, "%s", a->host);
    else snprintf(t->host_header, sizeof t->host_header, "%s:%d", a->host, a->port);

    struct lws_client_connect_info ci;
    memset(&ci, 0, sizeof ci);
    ci.context = t->ctx;
    ci.address = a->host;
    ci.port = a->port;
    ci.path = a->path;
    ci.host = t->host_header;
    ci.origin = NULL;   /* docs/websocket.md: send no Origin header */
    ci.protocol = NULL; /* no subprotocol */
    ci.local_protocol_name = ws_protocols[0].name;
    ci.ssl_connection = a->kind == ADDR_WSS ? LCCSCF_USE_SSL : 0;
    ci.pwsi = &t->wsi;
    if (!lws_client_connect_via_info(&ci)) {
        snprintf(err, errlen, "can't connect to %s%s%s", a->label, lws_last_error[0] ? ": " : "", lws_last_error);
        lws_context_destroy(t->ctx);
        t->ctx = NULL;
        free_transport(t);
        return NULL;
    }
    if (pthread_create(&t->thread, NULL, ws_thread, t) != 0) {
        snprintf(err, errlen, "can't start the network thread");
        lws_context_destroy(t->ctx);
        t->ctx = NULL;
        free_transport(t);
        return NULL;
    }
    t->thread_started = true;
    return t;
}

static int ws_send(transport *t, const uint8_t *frame, size_t len) {
    ws_out *o = malloc(sizeof *o + LWS_PRE + len);
    if (!o) return -1;
    o->next = NULL;
    o->len = len;
    memcpy(o->buf + LWS_PRE, frame, len);
    pthread_mutex_lock(&t->lock);
    if (t->state == TRANSPORT_CLOSED || t->closing) {
        pthread_mutex_unlock(&t->lock);
        free(o);
        return -1;
    }
    if (t->out_tail) t->out_tail->next = o;
    else t->out_head = o;
    t->out_tail = o;
    pthread_mutex_unlock(&t->lock);
    lws_cancel_service(t->ctx);
    return 0;
}

static void sleep_ms(int ms) {
    struct timespec ts = {ms / 1000, (long)(ms % 1000) * 1000000L};
    nanosleep(&ts, NULL);
}

static void ws_close(transport *t) {
    pthread_mutex_lock(&t->lock);
    bool open = t->state == TRANSPORT_OPEN;
    t->closing = true;
    pthread_mutex_unlock(&t->lock);
    if (open) {
        lws_cancel_service(t->ctx);
        for (int i = 0; i < 100; i++) { /* up to 1 s for queued frames and the close frame to go */
            pthread_mutex_lock(&t->lock);
            bool gone = t->wsi == NULL;
            pthread_mutex_unlock(&t->lock);
            if (gone) break;
            sleep_ms(10);
        }
    }
    t->stop = true;
    lws_cancel_service(t->ctx);
    if (t->thread_started) pthread_join(t->thread, NULL);
    lws_context_destroy(t->ctx);
    t->ctx = NULL;
}

/* ------------------------------------------------------------------------------------------------ */

transport *transport_open(const server_addr *addr, char *err, size_t errlen) {
    if (addr->kind == ADDR_TCP) {
        int fd = tcp_connect(addr, err, errlen);
        if (fd < 0) return NULL;
        transport *t = alloc_transport(ADDR_TCP);
        if (!t) {
            close(fd);
            snprintf(err, errlen, "out of memory");
            return NULL;
        }
        t->sock = fd;
        t->state = TRANSPORT_OPEN;
        return t;
    }
    return ws_open(addr, err, errlen);
}

int transport_fd(const transport *t) { return t->kind == ADDR_TCP ? t->sock : t->pair_reader; }

transport_state transport_get_state(transport *t, char *reason, size_t reasonlen) {
    pthread_mutex_lock(&t->lock);
    transport_state s = t->state;
    if (reason && reasonlen) snprintf(reason, reasonlen, "%s", t->reason);
    pthread_mutex_unlock(&t->lock);
    return s;
}

ssize_t transport_read(transport *t, uint8_t *buf, size_t cap) {
    ssize_t n;
    do {
        n = t->kind == ADDR_TCP ? recv(t->sock, buf, cap, MSG_DONTWAIT) : read(t->pair_reader, buf, cap);
    } while (n < 0 && errno == EINTR);
    if (t->kind == ADDR_TCP) {
        if (n == 0) set_closed(t, "the server closed the connection");
        else if (n < 0 && errno != EAGAIN && errno != EWOULDBLOCK) set_closed(t, strerror(errno));
    } else if (n == 0) {
        set_closed(t, "connection closed");
    }
    return n;
}

int transport_send_frame(transport *t, const uint8_t *frame, size_t len) {
    return t->kind == ADDR_TCP ? tcp_send(t, frame, len) : ws_send(t, frame, len);
}

void transport_close(transport *t) {
    if (!t) return;
    if (t->kind == ADDR_TCP) {
        if (t->sock >= 0) shutdown(t->sock, SHUT_RDWR);
    } else {
        ws_close(t);
    }
    free_transport(t);
}
