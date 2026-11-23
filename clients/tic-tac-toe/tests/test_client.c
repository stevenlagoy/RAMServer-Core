/* The client's session state, driven by hand-built sequences of server messages: the handshake,
 * joining, readying, moves, errors, match ends, rematches, leaving, heartbeats and disconnects. */
#include <stdlib.h>
#include <string.h>

#include "helpers.h"

#define ST_WAITING 1
#define ST_IN_PROGRESS 4
#define ST_CLOSED 5
#define E_SESSION_NOT_FOUND 4
#define E_OUT_OF_TURN 6
#define E_ILLEGAL 7
#define E_AUTH_FAILED 2
#define E_PROTOCOL 1
#define E_NOT_IN_SESSION 10
#define E_STALE 13
#define E_UNKNOWN_GAME 3
#define END_COMPLETED 1
#define END_FORFEIT 2

typedef Ramserver__V1__ClientMessage CM;

static const char *ME = "p-ada";
static const char *THEM = "p-bob";
static const char *SID = "s-1";
/* Ada joined first (X), Bob second (O) */
#define ROSTER_UNREADY "p-ada:Ada:player:0:1;p-bob:Bob:player:0:1"
#define ROSTER_ADA_READY "p-ada:Ada:player:1:1;p-bob:Bob:player:0:1"
#define ROSTER_READY "p-ada:Ada:player:1:1;p-bob:Bob:player:1:1"
#define ROSTER_PLAYING "p-ada:Ada:player:0:1;p-bob:Bob:player:0:1"
#define ANN_START "x_player=p-ada;o_player=p-bob;moves=0"

static ttt_client *new_client(void) {
    ttt_client *c = malloc(sizeof *c);
    client_init(c, "Ada", "test-client");
    return c;
}

static void destroy(ttt_client *c) {
    client_free(c);
    free(c);
}

/* Takes the next message, checks its payload type, returns it */
static CM *expect(ttt_client *c, int payload_case) {
    CM *m = take_cm(c);
    assert_non_null(m);
    assert_int_equal(m->payload_case, payload_case);
    return m;
}

/* Connects and signs in, returning the client in the lobby */
static ttt_client *signed_in(void) {
    ttt_client *c = new_client();
    client_connected(c);
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_CONNECT_REQUEST);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_connect_response(seq));
    m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_AUTH_REQUEST);
    seq = m->sequence;
    free_cm(m);
    feed(c, sm_auth_response(seq, ME, "tok-1", ""));
    assert_int_equal(c->phase, PHASE_LOBBY);
    return c;
}

/* Signed in, then joined SID by id, with Bob already there */
static ttt_client *in_session(void) {
    ttt_client *c = signed_in();
    assert_true(client_cmd_join(c, SID));
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_JOIN_SESSION_REQUEST);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_state(seq, SID, ST_WAITING, 3, ROSTER_UNREADY, NULL, NULL, NULL));
    assert_int_equal(c->phase, PHASE_SESSION);
    drain(c); /* the MatchHistoryRequest */
    return c;
}

/* In a match that just started (version 10), X to move */
static ttt_client *in_match(void) {
    ttt_client *c = in_session();
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 10, ROSTER_PLAYING, ".........XX", ME, ANN_START));
    return c;
}

/* ---- handshake ---- */

static void test_handshake(void **s) {
    (void)s;
    ttt_client *c = new_client();
    assert_int_equal(c->phase, PHASE_OFFLINE);
    client_connected(c);
    assert_int_equal(c->phase, PHASE_HANDSHAKE);
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_CONNECT_REQUEST);
    assert_int_equal(m->sequence, 1); /* sequences start at 1 */
    assert_string_equal(m->connect_request->protocol_version, "1.2");
    assert_null(take_cm(c));
    free_cm(m);

    feed(c, sm_connect_response(1));
    assert_int_equal(c->phase, PHASE_AUTH);
    m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_AUTH_REQUEST);
    assert_int_equal(m->sequence, 2);
    assert_string_equal(m->auth_request->display_name, "Ada");
    assert_string_equal(m->auth_request->player_id, ""); /* a new player */
    free_cm(m);

    feed(c, sm_auth_response(2, ME, "tok-1", ""));
    assert_int_equal(c->phase, PHASE_LOBBY);
    assert_string_equal(c->player_id, ME);
    assert_string_equal(c->reconnect_token, "tok-1");
    assert_null(take_cm(c)); /* nothing else until the player chooses */
    destroy(c);
}

static void test_version_mismatch_is_fatal(void **s) {
    (void)s;
    ttt_client *c = new_client();
    client_connected(c);
    drain(c);
    feed(c, sm_error(1, E_PROTOCOL, "server speaks 2.0"));
    assert_true(c->fatal);
    assert_true(log_has(c, "INVALID_PROTOCOL_VERSION"));
    assert_true(log_has(c, "server speaks 2.0"));
    destroy(c);
}

static void test_auto_join_after_sign_in(void **s) {
    (void)s;
    ttt_client *c = new_client();
    c->auto_join = JOIN_CREATE_PRIVATE;
    client_connected(c);
    drain(c);
    feed(c, sm_connect_response(1));
    drain(c);
    feed(c, sm_auth_response(2, ME, "tok", ""));
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_JOIN_SESSION_REQUEST);
    assert_true(m->join_session_request->create_new);
    assert_true(m->join_session_request->private_session);
    assert_string_equal(m->join_session_request->game_id, "tictactoe");
    free_cm(m);
    assert_int_equal(c->phase, PHASE_JOINING);
    destroy(c);
}

/* ---- finding a game ---- */

static void test_quick_match_and_join_reply(void **s) {
    (void)s;
    ttt_client *c = signed_in();
    assert_true(client_cmd_quick_match(c));
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_JOIN_SESSION_REQUEST);
    assert_string_equal(m->join_session_request->session_id, "");
    assert_false(m->join_session_request->create_new);
    uint64_t seq = m->sequence;
    free_cm(m);

    /* A broadcast for some other request doesn't count as the reply */
    feed(c, sm_state(0, "s-other", ST_WAITING, 1, "p-x:X:player:0:1", NULL, NULL, NULL));
    assert_int_equal(c->phase, PHASE_JOINING);

    feed(c, sm_state(seq, "s-new", ST_WAITING, 1, "p-ada:Ada:player:0:1", NULL, NULL, NULL));
    assert_int_equal(c->phase, PHASE_SESSION);
    assert_string_equal(c->session_id, "s-new");
    assert_int_equal(c->n_members, 1);
    assert_false(c->private_session);
    /* It asks for the session's history, for the scoreboard */
    m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_MATCH_HISTORY_REQUEST);
    assert_string_equal(m->match_history_request->session_id, "s-new");
    free_cm(m);
    destroy(c);
}

static void test_join_errors_return_to_lobby(void **s) {
    (void)s;
    ttt_client *c = signed_in();
    client_cmd_join(c, "nope");
    CM *m = take_cm(c);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_error(seq, E_SESSION_NOT_FOUND, "no such session"));
    assert_int_equal(c->phase, PHASE_LOBBY);
    assert_true(log_has(c, "SESSION_NOT_FOUND: no such session"));

    client_cmd_quick_match(c);
    m = take_cm(c);
    seq = m->sequence;
    free_cm(m);
    feed(c, sm_error(seq, E_UNKNOWN_GAME, "game: unknown game \"tictactoe\""));
    assert_int_equal(c->phase, PHASE_LOBBY);
    assert_true(log_has(c, "doesn't have tic-tac-toe"));
    destroy(c);
}

static void test_commands_refused_in_wrong_phase(void **s) {
    (void)s;
    ttt_client *c = new_client();
    assert_false(client_cmd_quick_match(c)); /* not signed in */
    assert_false(client_cmd_move(c, 0, true));
    assert_false(client_cmd_leave(c));
    assert_int_equal(client_outbox_count(c), 0);
    destroy(c);

    c = in_session();
    assert_false(client_cmd_quick_match(c)); /* already in a session */
    assert_false(client_cmd_join(c, ""));
    assert_false(client_cmd_move(c, 4, true)); /* no match yet */
    assert_int_equal(client_outbox_count(c), 0);
    destroy(c);
}

static void test_list_sessions(void **s) {
    (void)s;
    ttt_client *c = signed_in();
    assert_true(client_cmd_list_sessions(c));
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_LIST_SESSIONS_REQUEST);
    assert_string_equal(m->list_sessions_request->game_id, "tictactoe");
    free_cm(m);
    const char *ids[] = {"s-a", "s-b"};
    bool joinable[] = {true, false};
    feed(c, sm_list(2, ids, joinable));
    assert_true(c->listed_valid);
    assert_int_equal(c->n_listed, 2);
    assert_string_equal(c->listed[0].id, "s-a");
    assert_true(c->listed[0].joinable);
    assert_string_equal(c->listed[0].owner_name, "Owner");
    assert_false(c->listed[1].joinable);
    destroy(c);
}

/* ---- the lobby ---- */

static void test_ready_toggle_and_roster(void **s) {
    (void)s;
    ttt_client *c = in_session();
    assert_int_equal(c->n_members, 2);
    assert_string_equal(client_name_of(c, THEM), "Bob");
    assert_true(client_is_player(c));

    assert_true(client_cmd_toggle_ready(c));
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_SET_READY_REQUEST);
    assert_true(m->set_ready_request->ready);
    assert_string_equal(m->set_ready_request->session_id, SID);
    uint64_t seq = m->sequence;
    free_cm(m);
    /* Not ready until the server says so */
    assert_false(client_me(c)->ready);
    feed(c, sm_state(seq, SID, ST_WAITING, 4, ROSTER_ADA_READY, NULL, NULL, NULL));
    assert_true(client_me(c)->ready);
    assert_true(log_has(c, "You are ready"));

    assert_true(client_cmd_toggle_ready(c)); /* now withdraws */
    m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_SET_READY_REQUEST);
    assert_false(m->set_ready_request->ready);
    free_cm(m);

    /* A spectator joins, then Bob leaves */
    feed(c, sm_state(0, SID, ST_WAITING, 5, ROSTER_ADA_READY ";p-cy:Cy:spectator:0:1", NULL, NULL, NULL));
    assert_true(log_has(c, "Cy joined as spectator"));
    feed(c, sm_state(0, SID, ST_WAITING, 6, "p-ada:Ada:player:1:1;p-cy:Cy:spectator:0:1", NULL, NULL, NULL));
    assert_true(log_has(c, "Bob left"));
    assert_null(client_member(c, THEM));
    destroy(c);
}

/* ---- playing ---- */

static void test_match_start_and_move(void **s) {
    (void)s;
    ttt_client *c = in_session();
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 10, ROSTER_PLAYING, ".........XX", ME, ANN_START));
    assert_true(client_in_match(c));
    assert_true(client_my_turn(c));
    assert_int_equal(c->board.me, 'X');
    assert_string_equal(c->x_player, ME);
    assert_string_equal(c->o_player, THEM);
    assert_true(log_has(c, "Match started: you are X"));

    assert_true(client_cmd_move(c, 4, true));
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_ACTION_REQUEST);
    assert_string_equal(m->action_request->session_id, SID);
    assert_int_equal(m->action_request->encoded_action.len, 1);
    assert_int_equal(m->action_request->encoded_action.data[0], '4');
    assert_int_equal(m->action_request->expected_state_version, 10);
    uint64_t seq = m->sequence;
    free_cm(m);

    /* The board doesn't change until the server's update arrives */
    assert_int_equal(c->board.cells[4], '.');
    assert_false(client_cmd_move(c, 0, true)); /* waiting for the reply */
    feed(c, sm_state(seq, SID, ST_IN_PROGRESS, 11, ROSTER_PLAYING, "....X....OX", THEM,
                     "x_player=p-ada;o_player=p-bob;moves=1;last_move=4"));
    assert_int_equal(c->board.cells[4], 'X');
    assert_int_equal(c->last_move, 4);
    assert_int_equal(c->moves, 1);
    assert_false(client_my_turn(c));
    assert_string_equal(c->active_id, THEM);

    /* Not our turn: refused locally, nothing sent */
    assert_false(client_cmd_move(c, 0, true));
    assert_true(strstr(log_last(c), "Bob's turn") != NULL);
    assert_int_equal(client_outbox_count(c), 0);

    /* Bob plays; our turn again, but the center is taken */
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 12, ROSTER_PLAYING, "O...X....XX", ME, ANN_START ";last_move=0"));
    assert_true(client_my_turn(c));
    assert_false(client_cmd_move(c, 4, true));
    assert_true(strstr(log_last(c), "center is taken") != NULL);
    assert_false(client_cmd_move(c, 9, true));
    assert_int_equal(client_outbox_count(c), 0);
    destroy(c);
}

static void test_server_errors_on_moves_are_shown(void **s) {
    (void)s;
    ttt_client *c = in_match();
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 11, ROSTER_PLAYING, "....X....OX", THEM, ANN_START));
    /* Without the guard the move goes to the server, which refuses it */
    assert_true(client_cmd_move(c, 4, false));
    CM *m = take_cm(c);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_error(seq, E_OUT_OF_TURN, "not your turn"));
    assert_true(log_has(c, "OUT_OF_TURN: not your turn"));
    assert_int_equal(c->state_version, 11); /* unchanged */
    assert_int_equal(c->board.cells[4], 'X');

    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 12, ROSTER_PLAYING, "O...X....XX", ME, ANN_START));
    client_cmd_move(c, 4, false);
    m = take_cm(c);
    seq = m->sequence;
    free_cm(m);
    feed(c, sm_error(seq, E_ILLEGAL, "illegal action: cell 4 is already taken"));
    assert_true(log_has(c, "ILLEGAL_ACTION: illegal action: cell 4 is already taken"));

    client_cmd_move(c, 1, true);
    m = take_cm(c);
    seq = m->sequence;
    free_cm(m);
    feed(c, sm_error(seq, E_STALE, "expected_state_version 12, current 13"));
    assert_true(log_has(c, "board changed before your move arrived"));
    /* the pending move is answered, so the player can try again */
    assert_false(client_has_pending(c, REQ_ACTION));

    /* An unsolicited error is shown too */
    feed(c, sm_error(0, 14, "something broke"));
    assert_true(log_has(c, "INTERNAL: something broke"));
    destroy(c);
}

static void test_stale_and_foreign_updates_ignored(void **s) {
    (void)s;
    ttt_client *c = in_match();
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 12, ROSTER_PLAYING, "O...X....XX", ME, NULL));
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 11, ROSTER_PLAYING, "....X....OX", THEM, NULL)); /* older */
    assert_int_equal(c->state_version, 12);
    assert_int_equal(c->board.cells[0], 'O');
    feed(c, sm_state(0, "s-other", ST_IN_PROGRESS, 99, ROSTER_PLAYING, "XXXXXXXXXDX", ME, NULL));
    assert_int_equal(c->state_version, 12);
    assert_int_equal(c->board.cells[1], '.');
    destroy(c);
}

static void test_bad_view_is_reported_not_drawn(void **s) {
    (void)s;
    ttt_client *c = in_match();
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 11, ROSTER_PLAYING, "garbage", ME, NULL));
    assert_true(log_has(c, "Couldn't read the board"));
    assert_false(client_in_match(c));
    destroy(c);
}

static void test_win_then_rematch(void **s) {
    (void)s;
    ttt_client *c = in_match();
    /* X wins on the top row */
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 15, ROSTER_PLAYING, "XXXOO....xX", "",
                     "x_player=p-ada;o_player=p-bob;moves=5;last_move=2;winning_line=0,1,2"));
    assert_false(client_in_match(c));
    assert_true(c->has_line);
    assert_int_equal(c->line[2], 2);
    feed(c, sm_match_result(SID, ME, "THREE_IN_A_ROW", END_COMPLETED, 5));
    assert_true(c->has_result);
    assert_false(c->result.draw);
    assert_string_equal(c->result.winner, ME);
    assert_int_equal(c->result.turns, 5);
    assert_int_equal(client_wins(c, ME), 1);
    assert_int_equal(client_wins(c, THEM), 0);
    assert_true(log_has(c, "You win by three in a row"));
    assert_int_equal(c->results_received, 1);

    /* Back to the lobby: ready flags cleared, empty view, last board kept for display */
    feed(c, sm_state(0, SID, ST_WAITING, 16, ROSTER_UNREADY, NULL, NULL, NULL));
    assert_int_equal(c->status, ST_WAITING);
    assert_true(c->has_board);
    assert_false(c->board_live);
    assert_memory_equal(c->board.cells, "XXXOO....", 9);
    assert_true(c->has_result);
    assert_true(log_has(c, "rematch"));

    /* Rematch */
    assert_true(client_cmd_set_ready(c, true));
    drain(c);
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 18, ROSTER_PLAYING, ".........XX", ME, ANN_START));
    assert_true(client_in_match(c));
    assert_false(c->has_result); /* the banner goes when the new match starts */
    assert_false(c->has_line);
    assert_int_equal(c->last_move, -1);
    assert_int_equal(client_wins(c, ME), 1); /* the scoreboard stays */
    destroy(c);
}

/* If a server labels the game-ending update as waiting, the final board still shows, but isn't playable */
static void test_final_board_in_waiting_update(void **s) {
    (void)s;
    ttt_client *c = in_match();
    feed(c, sm_state(0, SID, ST_WAITING, 15, ROSTER_UNREADY, "XXXOO....xX", "", "winning_line=0,1,2"));
    assert_true(c->has_board);
    assert_false(c->board_live);
    assert_false(client_in_match(c));
    assert_true(c->has_line);
    assert_memory_equal(c->board.cells, "XXXOO....", 9);
    assert_false(client_cmd_move(c, 8, true));
    destroy(c);
}

static void test_loss_draw_and_resignation(void **s) {
    (void)s;
    ttt_client *c = in_match();
    feed(c, sm_match_result(SID, THEM, "THREE_IN_A_ROW", END_COMPLETED, 6));
    assert_true(log_has(c, "Bob wins by three in a row"));
    feed(c, sm_state(0, SID, ST_WAITING, 20, ROSTER_UNREADY, NULL, NULL, NULL));
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 22, ROSTER_PLAYING, ".........XX", ME, ANN_START));
    feed(c, sm_match_result(SID, NULL, "DRAW", END_COMPLETED, 9));
    assert_true(c->result.draw);
    assert_true(log_has(c, "Draw, by a full board, after 9 turns"));
    assert_int_equal(client_draws(c), 1);
    assert_int_equal(client_wins(c, THEM), 1);

    feed(c, sm_state(0, SID, ST_WAITING, 24, ROSTER_UNREADY, NULL, NULL, NULL));
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 26, ROSTER_PLAYING, ".........XX", ME, ANN_START));
    assert_true(client_cmd_resign(c));
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_ACTION_REQUEST);
    assert_int_equal(m->action_request->encoded_action.len, 6);
    assert_memory_equal(m->action_request->encoded_action.data, "resign", 6);
    free_cm(m);
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 27, ROSTER_PLAYING, ".........oX", "", ANN_START ";resigned=X"));
    assert_int_equal(c->resigned, 'X');
    feed(c, sm_match_result(SID, THEM, "RESIGNATION", END_COMPLETED, 1));
    assert_true(log_has(c, "Bob wins by resignation"));
    assert_int_equal(c->n_results, 3);
    destroy(c);
}

static void test_forfeit_by_server(void **s) {
    (void)s;
    ttt_client *c = in_match();
    feed(c, sm_match_result(SID, ME, "", END_FORFEIT, 3));
    assert_true(log_has(c, "You win by forfeit"));
    destroy(c);
}

static void test_spectator(void **s) {
    (void)s;
    ttt_client *c = signed_in();
    client_cmd_join(c, SID);
    CM *m = take_cm(c);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_state(seq, SID, ST_IN_PROGRESS, 30, "p-bob:Bob:player:0:1;p-cy:Cy:player:0:1;p-ada:Ada:spectator:0:1",
                     "....X....OS", "p-cy", NULL));
    drain(c);
    assert_false(client_is_player(c));
    assert_true(client_in_match(c));
    assert_false(client_my_turn(c));
    assert_false(client_cmd_move(c, 0, true));
    assert_true(log_has(c, "Spectators can't move"));
    assert_false(client_cmd_resign(c));
    assert_int_equal(client_outbox_count(c), 0);
    destroy(c);
}

static void test_match_history(void **s) {
    (void)s;
    ttt_client *c = signed_in();
    client_cmd_join(c, SID);
    CM *m = take_cm(c);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_state(seq, SID, ST_WAITING, 3, ROSTER_UNREADY, NULL, NULL, NULL));
    drain(c);
    const char *winners[] = {THEM, NULL, THEM};
    feed(c, sm_history(SID, 3, winners));
    assert_int_equal(c->n_results, 3);
    assert_int_equal(client_wins(c, THEM), 2);
    assert_int_equal(client_draws(c), 1);
    assert_true(log_has(c, "played 3 matches"));
    /* history for another session is ignored */
    feed(c, sm_history("s-other", 1, winners));
    assert_int_equal(c->n_results, 3);
    destroy(c);
}

/* ---- leaving, heartbeats, disconnects ---- */

static void test_leave(void **s) {
    (void)s;
    ttt_client *c = in_session();
    assert_true(client_cmd_leave(c));
    assert_int_equal(c->phase, PHASE_LEAVING);
    CM *m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_LEAVE_SESSION_REQUEST);
    assert_string_equal(m->leave_session_request->session_id, SID);
    uint64_t seq = m->sequence;
    free_cm(m);
    assert_false(client_cmd_leave(c)); /* already leaving */
    feed(c, sm_leave_response(seq, SID));
    assert_int_equal(c->phase, PHASE_LOBBY);
    assert_string_equal(c->session_id, "");
    assert_int_equal(c->n_members, 0);
    /* late broadcasts from the old session are ignored */
    feed(c, sm_state(0, SID, ST_WAITING, 50, ROSTER_UNREADY, NULL, NULL, NULL));
    assert_int_equal(c->n_members, 0);
    destroy(c);
}

static void test_leave_not_in_session(void **s) {
    (void)s;
    ttt_client *c = in_session();
    client_cmd_leave(c);
    CM *m = take_cm(c);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_error(seq, E_NOT_IN_SESSION, "not a member"));
    assert_int_equal(c->phase, PHASE_LOBBY);
    destroy(c);
}

static void test_session_closed(void **s) {
    (void)s;
    ttt_client *c = in_session();
    feed(c, sm_state(0, SID, ST_CLOSED, 9, ROSTER_UNREADY, NULL, NULL, NULL));
    assert_true(log_has(c, "session has closed"));
    destroy(c);
}

static void test_heartbeat(void **s) {
    (void)s;
    ttt_client *c = in_session();
    assert_false(client_take_heartbeat(c));
    assert_true(client_on_frame(c, NULL, 0));
    assert_true(client_take_heartbeat(c));
    assert_false(client_take_heartbeat(c)); /* once per heartbeat */
    assert_int_equal(client_outbox_count(c), 0); /* heartbeats aren't protobuf messages */
    destroy(c);
}

static void test_garbage_payload_ignored(void **s) {
    (void)s;
    ttt_client *c = in_session();
    const uint8_t junk[] = {0xff, 0xff, 0xff, 0xff, 0x0f};
    assert_false(client_on_frame(c, junk, sizeof junk));
    assert_true(log_has(c, "isn't a valid ServerMessage"));
    assert_int_equal(c->phase, PHASE_SESSION);
    destroy(c);
}

static void test_disconnect_and_resume_fallback(void **s) {
    (void)s;
    ttt_client *c = in_match();
    client_disconnected(c, "connection reset");
    assert_int_equal(c->phase, PHASE_OFFLINE);
    assert_false(client_in_match(c));
    assert_string_equal(c->session_id, "");
    assert_true(log_has(c, "Connection lost: connection reset"));

    /* Reconnect: tries to resume the old identity */
    client_connected(c);
    CM *m = take_cm(c);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_connect_response(seq));
    m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_AUTH_REQUEST);
    assert_string_equal(m->auth_request->player_id, ME);
    assert_string_equal(m->auth_request->reconnect_token, "tok-1");
    seq = m->sequence;
    free_cm(m);
    /* The server doesn't support resuming yet: sign in fresh */
    feed(c, sm_error(seq, E_AUTH_FAILED, "unknown player"));
    m = expect(c, RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_AUTH_REQUEST);
    assert_string_equal(m->auth_request->player_id, "");
    assert_string_equal(m->auth_request->display_name, "Ada");
    seq = m->sequence;
    free_cm(m);
    feed(c, sm_auth_response(seq, "p-ada-2", "tok-2", ""));
    assert_int_equal(c->phase, PHASE_LOBBY);
    assert_string_equal(c->player_id, "p-ada-2");
    destroy(c);
}

static void test_resume_into_session(void **s) {
    (void)s;
    ttt_client *c = in_session();
    client_disconnected(c, "dropped");
    client_connected(c);
    CM *m = take_cm(c);
    uint64_t seq = m->sequence;
    free_cm(m);
    feed(c, sm_connect_response(seq));
    m = take_cm(c);
    seq = m->sequence;
    free_cm(m);
    feed(c, sm_auth_response(seq, ME, "tok-1", SID));
    assert_int_equal(c->phase, PHASE_SESSION);
    assert_string_equal(c->session_id, SID);
    drain(c);
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 40, ROSTER_PLAYING, "X...O....XX", ME, ANN_START));
    assert_true(client_my_turn(c));
    destroy(c);
}

static void test_auth_refused_is_fatal(void **s) {
    (void)s;
    ttt_client *c = new_client();
    client_connected(c);
    drain(c);
    feed(c, sm_connect_response(1));
    drain(c);
    feed(c, sm_error(2, 12, "display_name too long"));
    assert_true(c->fatal);
    assert_true(log_has(c, "INVALID_ARGUMENT: display_name too long"));
    destroy(c);
}

static void test_unknown_annotations_ignored(void **s) {
    (void)s;
    ttt_client *c = in_session();
    feed(c, sm_state(0, SID, ST_IN_PROGRESS, 10, ROSTER_PLAYING, ".........XX", ME,
                     ANN_START ";future_key=whatever;last_move=bogus;winning_line=nope"));
    assert_true(client_in_match(c));
    assert_int_equal(c->last_move, -1);
    assert_false(c->has_line);
    destroy(c);
}

int main(void) {
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_handshake),
        cmocka_unit_test(test_version_mismatch_is_fatal),
        cmocka_unit_test(test_auto_join_after_sign_in),
        cmocka_unit_test(test_quick_match_and_join_reply),
        cmocka_unit_test(test_join_errors_return_to_lobby),
        cmocka_unit_test(test_commands_refused_in_wrong_phase),
        cmocka_unit_test(test_list_sessions),
        cmocka_unit_test(test_ready_toggle_and_roster),
        cmocka_unit_test(test_match_start_and_move),
        cmocka_unit_test(test_server_errors_on_moves_are_shown),
        cmocka_unit_test(test_stale_and_foreign_updates_ignored),
        cmocka_unit_test(test_bad_view_is_reported_not_drawn),
        cmocka_unit_test(test_win_then_rematch),
        cmocka_unit_test(test_final_board_in_waiting_update),
        cmocka_unit_test(test_loss_draw_and_resignation),
        cmocka_unit_test(test_forfeit_by_server),
        cmocka_unit_test(test_spectator),
        cmocka_unit_test(test_match_history),
        cmocka_unit_test(test_leave),
        cmocka_unit_test(test_leave_not_in_session),
        cmocka_unit_test(test_session_closed),
        cmocka_unit_test(test_heartbeat),
        cmocka_unit_test(test_garbage_payload_ignored),
        cmocka_unit_test(test_disconnect_and_resume_fallback),
        cmocka_unit_test(test_resume_into_session),
        cmocka_unit_test(test_auth_refused_is_fatal),
        cmocka_unit_test(test_unknown_annotations_ignored),
    };
    return cmocka_run_group_tests(tests, NULL, NULL);
}
