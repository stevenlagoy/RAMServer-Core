/* Parsing server addresses */
#include "addr.h"
#include "helpers.h"

static server_addr ok(const char *spec) {
    server_addr a;
    char err[128];
    assert_true(addr_parse(spec, &a, err, sizeof err));
    return a;
}

static void test_presets(void **s) {
    (void)s;
    server_addr a = ok("local");
    assert_int_equal(a.kind, ADDR_TCP);
    assert_string_equal(a.host, "127.0.0.1");
    assert_int_equal(a.port, 8080);
    a = ok("local-ws");
    assert_int_equal(a.kind, ADDR_WS);
    assert_int_equal(a.port, 8081);
    assert_string_equal(a.path, "/connect");
    a = ok("hosted");
    assert_int_equal(a.kind, ADDR_WSS);
    assert_string_equal(a.host, "ramserver-core-80752473398.us-east5.run.app");
    assert_int_equal(a.port, 443);
    assert_string_equal(a.path, "/connect");
    assert_string_equal(a.label, "wss://ramserver-core-80752473398.us-east5.run.app/connect");
}

static void test_forms(void **s) {
    (void)s;
    server_addr a = ok("host.docker.internal:8080");
    assert_int_equal(a.kind, ADDR_TCP);
    assert_string_equal(a.host, "host.docker.internal");
    assert_string_equal(a.label, "tcp://host.docker.internal:8080");
    a = ok("tcp://10.0.0.5:9000");
    assert_int_equal(a.port, 9000);
    a = ok("[::1]:8080");
    assert_string_equal(a.host, "::1");
    assert_string_equal(a.label, "tcp://[::1]:8080");
    a = ok("ws://example.com");
    assert_int_equal(a.port, 80);
    assert_string_equal(a.path, "/connect");
    a = ok("wss://example.com:8443/other");
    assert_int_equal(a.port, 8443);
    assert_string_equal(a.path, "/other");
    assert_string_equal(a.label, "wss://example.com:8443/other");
}

static void test_errors(void **s) {
    (void)s;
    const char *bad[] = {"", "localhost", "host:", "host:0", "host:70000", "host:12a", "http://x", "ws://", "[::1", "tcp://h:1/path"};
    for (size_t i = 0; i < sizeof bad / sizeof bad[0]; i++) {
        server_addr a;
        char err[128] = "";
        assert_false(addr_parse(bad[i], &a, err, sizeof err));
        assert_true(err[0] != '\0');
    }
}

int main(void) {
    const struct CMUnitTest tests[] = {cmocka_unit_test(test_presets), cmocka_unit_test(test_forms), cmocka_unit_test(test_errors)};
    return cmocka_run_group_tests(tests, NULL, NULL);
}
