package stats

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type RangeMode string

const (
	ModeDay RangeMode = "day"
	ModeAll RangeMode = "all"
)

type DateRange struct {
	Mode      RangeMode
	Start     time.Time
	End       time.Time
	Label     string
	StartDate string
	EndDate   string
}

type Event struct {
	EventTime        time.Time
	EventDate        string
	SessionType      string
	SessionID        int64
	Direction        string
	SourceType       string
	BotName          string
	UserID           int64
	NicknameSnapshot string
	MessageType      string
	ActionName       string
}

type SessionSummary struct {
	RecvCount         int64
	SendCount         int64
	BotSendCount      int64
	InternalSendCount int64
}

type UserRank struct {
	UserID    int64
	RecvCount int64
	Snapshot  string
}

type BotSendRank struct {
	BotName   string
	SendCount int64
}

type SessionTraffic struct {
	SessionType        string
	SessionID          int64
	RecvCount          int64
	SendCount          int64
	BotSendCount       int64
	InternalSendCount  int64
}

type Store struct {
	db *sql.DB
}

func OpenStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`CREATE TABLE IF NOT EXISTS stats_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_time INTEGER NOT NULL,
			event_date TEXT NOT NULL,
			session_type TEXT NOT NULL,
			session_id INTEGER NOT NULL,
			direction TEXT NOT NULL,
			source_type TEXT NOT NULL,
			bot_name TEXT NOT NULL DEFAULT '',
			user_id INTEGER NOT NULL DEFAULT 0,
			nickname_snapshot TEXT NOT NULL DEFAULT '',
			message_type TEXT NOT NULL DEFAULT '',
			action_name TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS stats_daily_session (
			stat_date TEXT NOT NULL,
			session_type TEXT NOT NULL,
			session_id INTEGER NOT NULL,
			recv_count INTEGER NOT NULL DEFAULT 0,
			send_count INTEGER NOT NULL DEFAULT 0,
			bot_send_count INTEGER NOT NULL DEFAULT 0,
			internal_send_count INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (stat_date, session_type, session_id)
		);`,
		`CREATE TABLE IF NOT EXISTS stats_daily_user_rank (
			stat_date TEXT NOT NULL,
			session_type TEXT NOT NULL,
			session_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			recv_count INTEGER NOT NULL DEFAULT 0,
			last_nickname_snapshot TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (stat_date, session_type, session_id, user_id)
		);`,
		`CREATE TABLE IF NOT EXISTS stats_daily_bot_send (
			stat_date TEXT NOT NULL,
			session_type TEXT NOT NULL,
			session_id INTEGER NOT NULL,
			bot_name TEXT NOT NULL,
			send_count INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (stat_date, session_type, session_id, bot_name)
		);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RecordBatch(events []Event) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, event := range events {
		if _, err = tx.Exec(
			`INSERT INTO stats_events (
				event_time, event_date, session_type, session_id, direction, source_type,
				bot_name, user_id, nickname_snapshot, message_type, action_name
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			event.EventTime.Unix(),
			event.EventDate,
			event.SessionType,
			event.SessionID,
			event.Direction,
			event.SourceType,
			event.BotName,
			event.UserID,
			event.NicknameSnapshot,
			event.MessageType,
			event.ActionName,
		); err != nil {
			return err
		}

		if _, err = tx.Exec(
			`INSERT INTO stats_daily_session (
				stat_date, session_type, session_id, recv_count, send_count, bot_send_count, internal_send_count
			) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(stat_date, session_type, session_id) DO UPDATE SET
				recv_count = recv_count + excluded.recv_count,
				send_count = send_count + excluded.send_count,
				bot_send_count = bot_send_count + excluded.bot_send_count,
				internal_send_count = internal_send_count + excluded.internal_send_count`,
			event.EventDate,
			event.SessionType,
			event.SessionID,
			boolToInt64(event.Direction == "recv"),
			boolToInt64(event.Direction == "send"),
			boolToInt64(event.Direction == "send" && event.SourceType == "bot_app"),
			boolToInt64(event.Direction == "send" && event.SourceType == "filter_internal"),
		); err != nil {
			return err
		}

		if event.Direction == "recv" && event.UserID > 0 {
			if _, err = tx.Exec(
				`INSERT INTO stats_daily_user_rank (
					stat_date, session_type, session_id, user_id, recv_count, last_nickname_snapshot
				) VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(stat_date, session_type, session_id, user_id) DO UPDATE SET
					recv_count = recv_count + excluded.recv_count,
					last_nickname_snapshot = excluded.last_nickname_snapshot`,
				event.EventDate,
				event.SessionType,
				event.SessionID,
				event.UserID,
				1,
				event.NicknameSnapshot,
			); err != nil {
				return err
			}
		}

		if event.Direction == "send" && event.SourceType == "bot_app" && event.BotName != "" {
			if _, err = tx.Exec(
				`INSERT INTO stats_daily_bot_send (
					stat_date, session_type, session_id, bot_name, send_count
				) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(stat_date, session_type, session_id, bot_name) DO UPDATE SET
					send_count = send_count + excluded.send_count`,
				event.EventDate,
				event.SessionType,
				event.SessionID,
				event.BotName,
				1,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *Store) QuerySessionSummary(sessionType string, sessionID int64, r DateRange) (SessionSummary, error) {
	if r.Mode == ModeAll || (r.StartDate == "" && r.EndDate == "") {
		return s.querySummaryAll(sessionType, sessionID)
	}

	row := s.db.QueryRow(
		`SELECT
			COALESCE(SUM(recv_count), 0),
			COALESCE(SUM(send_count), 0),
			COALESCE(SUM(bot_send_count), 0),
			COALESCE(SUM(internal_send_count), 0)
		FROM stats_daily_session
		WHERE session_type = ? AND session_id = ? AND stat_date BETWEEN ? AND ?`,
		sessionType,
		sessionID,
		r.StartDate,
		r.EndDate,
	)

	var summary SessionSummary
	err := row.Scan(&summary.RecvCount, &summary.SendCount, &summary.BotSendCount, &summary.InternalSendCount)
	return summary, err
}

func (s *Store) QueryUserRank(sessionType string, sessionID int64, r DateRange, limit int) ([]UserRank, error) {
	if limit <= 0 {
		limit = 15
	}

	where, args := rangeWhere(sessionType, sessionID, r)
	args = append(args, limit)

	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT user_id, SUM(recv_count) AS total_count, MAX(last_nickname_snapshot)
FROM stats_daily_user_rank
WHERE %s
GROUP BY user_id
ORDER BY total_count DESC, user_id ASC
LIMIT ?`, where),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UserRank
	for rows.Next() {
		var rank UserRank
		if err := rows.Scan(&rank.UserID, &rank.RecvCount, &rank.Snapshot); err != nil {
			return nil, err
		}
		result = append(result, rank)
	}
	return result, rows.Err()
}

func (s *Store) QueryBotSend(sessionType string, sessionID int64, r DateRange) ([]BotSendRank, error) {
	where, args := rangeWhere(sessionType, sessionID, r)
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT bot_name, SUM(send_count) AS total_count
FROM stats_daily_bot_send
WHERE %s
GROUP BY bot_name
ORDER BY total_count DESC, bot_name ASC`, where),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []BotSendRank
	for rows.Next() {
		var rank BotSendRank
		if err := rows.Scan(&rank.BotName, &rank.SendCount); err != nil {
			return nil, err
		}
		result = append(result, rank)
	}
	return result, rows.Err()
}

func (s *Store) QueryGlobalBotSend(r DateRange) ([]BotSendRank, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if r.Mode == ModeAll || (r.StartDate == "" && r.EndDate == "") {
		rows, err = s.db.Query(
			`SELECT bot_name, SUM(send_count) AS total_count
FROM stats_daily_bot_send
GROUP BY bot_name
ORDER BY total_count DESC, bot_name ASC`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT bot_name, SUM(send_count) AS total_count
FROM stats_daily_bot_send
WHERE stat_date BETWEEN ? AND ?
GROUP BY bot_name
ORDER BY total_count DESC, bot_name ASC`,
			r.StartDate,
			r.EndDate,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []BotSendRank
	for rows.Next() {
		var rank BotSendRank
		if err := rows.Scan(&rank.BotName, &rank.SendCount); err != nil {
			return nil, err
		}
		result = append(result, rank)
	}
	return result, rows.Err()
}

func (s *Store) QueryGlobalSummary(r DateRange) (SessionSummary, error) {
	if r.Mode == ModeAll || (r.StartDate == "" && r.EndDate == "") {
		row := s.db.QueryRow(
			`SELECT
				COALESCE(SUM(recv_count), 0),
				COALESCE(SUM(send_count), 0),
				COALESCE(SUM(bot_send_count), 0),
				COALESCE(SUM(internal_send_count), 0)
			FROM stats_daily_session`,
		)
		var summary SessionSummary
		err := row.Scan(&summary.RecvCount, &summary.SendCount, &summary.BotSendCount, &summary.InternalSendCount)
		return summary, err
	}

	row := s.db.QueryRow(
		`SELECT
			COALESCE(SUM(recv_count), 0),
			COALESCE(SUM(send_count), 0),
			COALESCE(SUM(bot_send_count), 0),
			COALESCE(SUM(internal_send_count), 0)
		FROM stats_daily_session
		WHERE stat_date BETWEEN ? AND ?`,
		r.StartDate,
		r.EndDate,
	)
	var summary SessionSummary
	err := row.Scan(&summary.RecvCount, &summary.SendCount, &summary.BotSendCount, &summary.InternalSendCount)
	return summary, err
}

func (s *Store) QuerySessionTraffic(r DateRange) ([]SessionTraffic, error) {
	var (
		rows *sql.Rows
		err  error
	)

	query := `SELECT
			session_type,
			session_id,
			COALESCE(SUM(recv_count), 0),
			COALESCE(SUM(send_count), 0),
			COALESCE(SUM(bot_send_count), 0),
			COALESCE(SUM(internal_send_count), 0)
		FROM stats_daily_session`
	if r.Mode == ModeAll || (r.StartDate == "" && r.EndDate == "") {
		rows, err = s.db.Query(query + `
		GROUP BY session_type, session_id
		ORDER BY (SUM(recv_count) + SUM(send_count)) DESC, session_type ASC, session_id ASC`)
	} else {
		rows, err = s.db.Query(query+`
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY session_type, session_id
		ORDER BY (SUM(recv_count) + SUM(send_count)) DESC, session_type ASC, session_id ASC`,
			r.StartDate, r.EndDate,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionTraffic
	for rows.Next() {
		var row SessionTraffic
		if err := rows.Scan(
			&row.SessionType,
			&row.SessionID,
			&row.RecvCount,
			&row.SendCount,
			&row.BotSendCount,
			&row.InternalSendCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) querySummaryAll(sessionType string, sessionID int64) (SessionSummary, error) {
	row := s.db.QueryRow(
		`SELECT
			COALESCE(SUM(recv_count), 0),
			COALESCE(SUM(send_count), 0),
			COALESCE(SUM(bot_send_count), 0),
			COALESCE(SUM(internal_send_count), 0)
		FROM stats_daily_session
		WHERE session_type = ? AND session_id = ?`,
		sessionType,
		sessionID,
	)
	var summary SessionSummary
	err := row.Scan(&summary.RecvCount, &summary.SendCount, &summary.BotSendCount, &summary.InternalSendCount)
	return summary, err
}

func rangeWhere(sessionType string, sessionID int64, r DateRange) (string, []any) {
	if r.Mode == ModeAll || (r.StartDate == "" && r.EndDate == "") {
		return "session_type = ? AND session_id = ?", []any{sessionType, sessionID}
	}
	return "session_type = ? AND session_id = ? AND stat_date BETWEEN ? AND ?", []any{sessionType, sessionID, r.StartDate, r.EndDate}
}

func boolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
