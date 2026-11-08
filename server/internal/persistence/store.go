package persistence

import (
	"context"
	"fmt"
	"time"
)

func (s *SQLiteStore) SaveMatch(ctx context.Context, record MatchRecord) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("persistence: begin match transaction: %w", err)
	}
	defer tx.Rollback()

	if record.SessionID == "" || record.GameID == "" {
		return 0, fmt.Errorf("persistence: missing session or game ID")
	}

	if record.SessionCreatedAt.IsZero() {
		return 0, fmt.Errorf("persistence: missing session creation time")
	}

	_, err = tx.ExecContext(ctx, `
        INSERT INTO session (session_id, game_id, created_at)
        VALUES (?, ?, ?)
        ON CONFLICT(session_id) DO NOTHING
    `,
		record.SessionID,
		record.GameID,
		record.SessionCreatedAt.UTC().Format(time.RFC3339Nano),
	)

	if err != nil {
		return 0, fmt.Errorf("persistence: save session: %w", err)
	}

	var existingGameID string

	err = tx.QueryRowContext(ctx, `
    SELECT game_id
    FROM session
    WHERE session_id = ?
	`, record.SessionID).Scan(&existingGameID)

	if err != nil {
		return 0, fmt.Errorf(
			"persistence: verify session game: %w", err,
		)
	}

	if existingGameID != record.GameID {
		return 0, fmt.Errorf(
			"persistence: session %s belongs to game %s, not %s",
			record.SessionID,
			existingGameID,
			record.GameID,
		)
	}

	createdAt := time.Now().UTC().Format(time.RFC3339Nano)

	for _, participant := range record.Participants {
		if participant.ClientID == "" || participant.DisplayName == "" {
			return 0, fmt.Errorf("persistence: participant missing ID or name")
		}

		_, err := tx.ExecContext(ctx, `
            INSERT INTO client (client_id, display_name, created_at)
            VALUES (?, ?, ?)
            ON CONFLICT(client_id) DO NOTHING
        `,
			participant.ClientID,
			participant.DisplayName,
			createdAt,
		)

		if err != nil {
			return 0, fmt.Errorf("persistence: save client %s: %w",
				participant.ClientID, err)
		}
	}

	if record.MatchNumber <= 0 {
		return 0, fmt.Errorf("persistence: invalid match number")
	}

	if record.StartedAt.IsZero() || record.EndedAt.IsZero() {
		return 0, fmt.Errorf("persistence: missing match timestamps")
	}

	if record.TerminationReason == "" {
		return 0, fmt.Errorf("persistence: missing termination reason")
	}

	result, err := tx.ExecContext(ctx, `
    INSERT INTO "match" (
        session_id,
        match_number,
        game_id,
        seed,
        started_at,
        ended_at,
        termination_reason,
        is_draw,
        action_count
    )
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.SessionID,
		record.MatchNumber,
		record.GameID,
		record.Seed,
		record.StartedAt.UTC().Format(time.RFC3339Nano),
		record.EndedAt.UTC().Format(time.RFC3339Nano),
		record.TerminationReason,
		record.IsDraw,
		len(record.Actions),
	)

	if err != nil {
		return 0, fmt.Errorf("persistence: save match: %w", err)
	}

	matchID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("persistence: get match ID: %w", err)
	}

	for _, participant := range record.Participants {
		if participant.Role == "" {
			return 0, fmt.Errorf(
				"persistence: participant %s missing role",
				participant.ClientID,
			)
		}

		_, err := tx.ExecContext(ctx, `
        INSERT INTO match_participant (
            match_id,
            client_id,
            role,
            seat,
            is_winner,
            score,
            actions_accepted,
            actions_rejected
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `,
			matchID,
			participant.ClientID,
			participant.Role,
			participant.Seat,
			participant.IsWinner,
			participant.Score,
			participant.ActionsAccepted,
			participant.ActionsRejected,
		)

		if err != nil {
			return 0, fmt.Errorf(
				"persistence: save participant %s: %w",
				participant.ClientID, err,
			)
		}
	}

	for i, action := range record.Actions {
		expectedSeq := int64(i + 1)

		if action.Seq != expectedSeq {
			return 0, fmt.Errorf(
				"persistence: expected action sequence %d, got %d",
				expectedSeq,
				action.Seq,
			)
		}

		if action.ClientID == "" {
			return 0, fmt.Errorf("persistence: action missing client ID")
		}

		if action.ReceivedAt.IsZero() {
			return 0, fmt.Errorf("persistence: action missing timestamp")
		}

		if action.Payload == nil {
			return 0, fmt.Errorf("persistence: action missing payload")
		}

		_, err := tx.ExecContext(ctx, `
        INSERT INTO action_log (
            match_id,
            seq,
            client_id,
            received_at,
            payload,
            outcome,
            rejection_code,
            rejection_reason,
            state_version
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `,
			matchID,
			action.Seq,
			action.ClientID,
			action.ReceivedAt.UTC().Format(time.RFC3339Nano),
			action.Payload,
			action.Outcome,
			action.RejectionCode,
			action.RejectionReason,
			action.StateVersion,
		)

		if err != nil {
			return 0, fmt.Errorf(
				"persistence: save action %d: %w",
				action.Seq, err,
			)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("persistence: commit match: %w", err)
	}

	return matchID, nil

}

func (s *SQLiteStore) GetMatch(ctx context.Context, id int64) (MatchRecord, error) {
	var record MatchRecord
	var sessionCreated, started, ended string

	// Retrieve match metadata.
	err := s.db.QueryRowContext(ctx, `
        SELECT
            m.match_id,
            m.session_id,
            s.created_at,
            m.match_number,
            m.game_id,
            m.seed,
            m.started_at,
            m.ended_at,
            m.termination_reason,
            m.is_draw
        FROM "match" AS m
        JOIN session AS s ON s.session_id = m.session_id
        WHERE m.match_id = ?
    `, id).Scan(
		&record.ID,
		&record.SessionID,
		&sessionCreated,
		&record.MatchNumber,
		&record.GameID,
		&record.Seed,
		&started,
		&ended,
		&record.TerminationReason,
		&record.IsDraw,
	)

	if err != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: retrieve match %d: %w", id, err,
		)
	}

	// Convert timestamps.
	record.SessionCreatedAt, err = time.Parse(time.RFC3339Nano, sessionCreated)
	if err != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: parse session timestamp: %w", err,
		)
	}

	record.StartedAt, err = time.Parse(time.RFC3339Nano, started)
	if err != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: parse match start: %w", err,
		)
	}

	record.EndedAt, err = time.Parse(time.RFC3339Nano, ended)
	if err != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: parse match end: %w", err,
		)
	}

	// Retrieve match participants.
	rows, err := s.db.QueryContext(ctx, `
        SELECT
            mp.client_id,
            c.display_name,
            mp.role,
            mp.seat,
            mp.is_winner,
            mp.score,
            mp.actions_accepted,
            mp.actions_rejected
        FROM match_participant AS mp
        JOIN client AS c ON c.client_id = mp.client_id
        WHERE mp.match_id = ?
        ORDER BY mp.seat
    `, id)

	if err != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: retrieve participants: %w", err,
		)
	}

	for rows.Next() {
		var participant ParticipantRecord

		err := rows.Scan(
			&participant.ClientID,
			&participant.DisplayName,
			&participant.Role,
			&participant.Seat,
			&participant.IsWinner,
			&participant.Score,
			&participant.ActionsAccepted,
			&participant.ActionsRejected,
		)

		if err != nil {
			rows.Close()
			return MatchRecord{}, fmt.Errorf(
				"persistence: scan participant: %w", err,
			)
		}

		record.Participants = append(record.Participants, participant)
	}

	readErr := rows.Err()
	closeErr := rows.Close()

	if readErr != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: read participants: %w", readErr,
		)
	}

	if closeErr != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: close participant rows: %w", closeErr,
		)
	}

	// Retrieve match actions in sequence order.
	rows, err = s.db.QueryContext(ctx, `
        SELECT
            seq,
            client_id,
            received_at,
            payload,
            outcome,
            COALESCE(rejection_code, ''),
            COALESCE(rejection_reason, ''),
            state_version
        FROM action_log
        WHERE match_id = ?
        ORDER BY seq
    `, id)

	if err != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: retrieve actions: %w", err,
		)
	}

	for rows.Next() {
		var action ActionRecord
		var receivedAt string

		err := rows.Scan(
			&action.Seq,
			&action.ClientID,
			&receivedAt,
			&action.Payload,
			&action.Outcome,
			&action.RejectionCode,
			&action.RejectionReason,
			&action.StateVersion,
		)

		if err != nil {
			rows.Close()
			return MatchRecord{}, fmt.Errorf(
				"persistence: scan action: %w", err,
			)
		}

		action.ReceivedAt, err = time.Parse(
			time.RFC3339Nano, receivedAt,
		)

		if err != nil {
			rows.Close()
			return MatchRecord{}, fmt.Errorf(
				"persistence: parse action timestamp: %w", err,
			)
		}

		record.Actions = append(record.Actions, action)
	}

	readErr = rows.Err()
	closeErr = rows.Close()

	if readErr != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: read actions: %w", readErr,
		)
	}

	if closeErr != nil {
		return MatchRecord{}, fmt.Errorf(
			"persistence: close action rows: %w", closeErr,
		)
	}

	return record, nil
}
