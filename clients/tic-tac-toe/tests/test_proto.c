/* Every request the client sends, built and then decoded again as the server would. */
#include <stdlib.h>
#include <string.h>

#include "helpers.h"
#include "proto.h"

typedef Ramserver__V1__ClientMessage CM;

static CM *unpack(ttt_bytes b) {
    assert_non_null(b.data);
    CM *m = ramserver__v1__client_message__unpack(NULL, b.len, b.data);
    free(b.data);
    assert_non_null(m);
    return m;
}

static void test_connect(void **s) {
    (void)s;
    CM *m = unpack(proto_connect(1, "1.2", "ramserver-ttt-c/1.0"));
    assert_int_equal(m->sequence, 1);
    assert_int_equal(m->payload_case, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_CONNECT_REQUEST);
    assert_string_equal(m->connect_request->protocol_version, "1.2");
    assert_string_equal(m->connect_request->client_name, "ramserver-ttt-c/1.0");
    free_cm(m);
}

static void test_auth_new_and_resume(void **s) {
    (void)s;
    CM *m = unpack(proto_auth(2, "Ada", NULL, NULL));
    assert_int_equal(m->sequence, 2);
    assert_int_equal(m->payload_case, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_AUTH_REQUEST);
    assert_string_equal(m->auth_request->display_name, "Ada");
    assert_string_equal(m->auth_request->player_id, "");
    assert_string_equal(m->auth_request->reconnect_token, "");
    free_cm(m);
    m = unpack(proto_auth(3, "Ada", "p-1", "secret"));
    assert_string_equal(m->auth_request->player_id, "p-1");
    assert_string_equal(m->auth_request->reconnect_token, "secret");
    free_cm(m);
}

static void test_join_forms(void **s) {
    (void)s;
    CM *m = unpack(proto_join(4, "tictactoe", NULL, false, false)); /* quick match */
    assert_int_equal(m->payload_case, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_JOIN_SESSION_REQUEST);
    assert_string_equal(m->join_session_request->game_id, "tictactoe");
    assert_string_equal(m->join_session_request->session_id, "");
    assert_false(m->join_session_request->create_new);
    assert_false(m->join_session_request->private_session);
    free_cm(m);
    m = unpack(proto_join(5, "tictactoe", NULL, true, true)); /* private */
    assert_true(m->join_session_request->create_new);
    assert_true(m->join_session_request->private_session);
    free_cm(m);
    m = unpack(proto_join(6, "tictactoe", "s-42", false, false)); /* by id */
    assert_string_equal(m->join_session_request->session_id, "s-42");
    assert_false(m->join_session_request->create_new);
    free_cm(m);
}

static void test_leave_ready(void **s) {
    (void)s;
    CM *m = unpack(proto_leave(7, "s-42"));
    assert_int_equal(m->payload_case, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_LEAVE_SESSION_REQUEST);
    assert_string_equal(m->leave_session_request->session_id, "s-42");
    free_cm(m);
    m = unpack(proto_ready(8, "s-42", true));
    assert_int_equal(m->payload_case, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_SET_READY_REQUEST);
    assert_string_equal(m->set_ready_request->session_id, "s-42");
    assert_true(m->set_ready_request->ready);
    free_cm(m);
    m = unpack(proto_ready(9, "s-42", false));
    assert_false(m->set_ready_request->ready);
    free_cm(m);
}

static void test_action(void **s) {
    (void)s;
    const uint8_t move[1] = {'4'};
    CM *m = unpack(proto_action(10, "s-42", move, 1, 17));
    assert_int_equal(m->payload_case, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_ACTION_REQUEST);
    assert_string_equal(m->action_request->session_id, "s-42");
    assert_int_equal(m->action_request->encoded_action.len, 1);
    assert_int_equal(m->action_request->encoded_action.data[0], '4');
    assert_int_equal(m->action_request->expected_state_version, 17);
    free_cm(m);
    m = unpack(proto_action(11, "s-42", (const uint8_t *)"resign", 6, 0));
    assert_int_equal(m->action_request->encoded_action.len, 6);
    assert_memory_equal(m->action_request->encoded_action.data, "resign", 6);
    assert_int_equal(m->action_request->expected_state_version, 0);
    free_cm(m);
}

static void test_list_and_history(void **s) {
    (void)s;
    CM *m = unpack(proto_list_sessions(12, "tictactoe"));
    assert_int_equal(m->payload_case, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_LIST_SESSIONS_REQUEST);
    assert_string_equal(m->list_sessions_request->game_id, "tictactoe");
    free_cm(m);
    m = unpack(proto_match_history(13, "s-42"));
    assert_int_equal(m->payload_case, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_MATCH_HISTORY_REQUEST);
    assert_string_equal(m->match_history_request->session_id, "s-42");
    free_cm(m);
}

static void test_names(void **s) {
    (void)s;
    assert_string_equal(proto_error_name(6), "OUT_OF_TURN");
    assert_string_equal(proto_error_name(14), "INTERNAL");
    assert_string_equal(proto_error_name(99), "UNKNOWN");
    assert_string_equal(proto_status_name(4), "IN_PROGRESS");
    assert_string_equal(proto_end_reason_name(2), "FORFEIT");
    assert_string_equal(proto_end_reason_name(-1), "UNKNOWN");
}

int main(void) {
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_connect),     cmocka_unit_test(test_auth_new_and_resume),
        cmocka_unit_test(test_join_forms),  cmocka_unit_test(test_leave_ready),
        cmocka_unit_test(test_action),      cmocka_unit_test(test_list_and_history),
        cmocka_unit_test(test_names),
    };
    return cmocka_run_group_tests(tests, NULL, NULL);
}
