package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go-forum/internal/forum"

	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS members (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	login_name TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	member_id INTEGER NOT NULL REFERENCES members(id),
	expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS boards (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT NOT NULL,
	sort INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS threads (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	board_id INTEGER NOT NULL REFERENCES boards(id),
	title TEXT NOT NULL,
	author_id INTEGER NOT NULL REFERENCES members(id),
	created_at TEXT NOT NULL,
	last_post_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS posts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	thread_id INTEGER NOT NULL REFERENCES threads(id),
	author_id INTEGER NOT NULL REFERENCES members(id),
	floor INTEGER NOT NULL,
	body_markdown TEXT NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE(thread_id, floor)
);

CREATE INDEX IF NOT EXISTS idx_threads_board_last ON threads(board_id, last_post_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_thread_floor ON posts(thread_id, floor);
CREATE INDEX IF NOT EXISTS idx_sessions_member ON sessions(member_id);

CREATE TABLE IF NOT EXISTS invite_codes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	code TEXT NOT NULL UNIQUE,
	issued_by INTEGER NOT NULL REFERENCES members(id),
	issued_at TEXT NOT NULL,
	revoked INTEGER NOT NULL DEFAULT 0,
	used_by_login TEXT,
	used_at TEXT
	);

	CREATE TABLE IF NOT EXISTS post_edits (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER NOT NULL REFERENCES posts(id),
		body_markdown TEXT NOT NULL,
		edited_at TEXT NOT NULL
	);
`

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE posts ADD COLUMN edited_at TEXT`,
		`ALTER TABLE posts ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE threads ADD COLUMN title_edited_at TEXT`,
		`ALTER TABLE threads ADD COLUMN pin_rank INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE boards ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE members ADD COLUMN suspended INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				_ = db.Close()
				return nil, fmt.Errorf("migrate: %w", err)
			}
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func parseTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, v)
	}
	return t
}

func (s *Store) EnsureFounder(loginName, displayName, passwordHash string) (*forum.Member, error) {
	if !forum.ValidLoginName(loginName) {
		return nil, forum.ErrInvalidLoginName
	}
	displayName, err := forum.NormalizeDisplayName(displayName)
	if err != nil {
		return nil, err
	}
	m, _, err := s.MemberByLogin(loginName)
	if err == nil {
		return m, nil
	}
	if err != forum.ErrNotFound {
		return nil, err
	}
	res, err := s.db.Exec(
		`INSERT INTO members (login_name, display_name, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		loginName, displayName, passwordHash, string(forum.RoleFounder), now(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert founder: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.MemberByID(id)
}

func (s *Store) MemberByLogin(loginName string) (*forum.Member, string, error) {
	var m forum.Member
	var hash, created, role string
	var suspended int
	err := s.db.QueryRow(
		`SELECT id, login_name, display_name, password_hash, role, suspended, created_at FROM members WHERE login_name = ?`,
		loginName,
	).Scan(&m.ID, &m.LoginName, &m.DisplayName, &hash, &role, &suspended, &created)
	if err == sql.ErrNoRows {
		return nil, "", forum.ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	m.Role = forum.Role(role)
	m.Suspended = suspended != 0
	m.CreatedAt = parseTime(created)
	return &m, hash, nil
}

func (s *Store) MemberByID(id int64) (*forum.Member, error) {
	var m forum.Member
	var created, role string
	var suspended int
	err := s.db.QueryRow(
		`SELECT id, login_name, display_name, role, suspended, created_at FROM members WHERE id = ?`,
		id,
	).Scan(&m.ID, &m.LoginName, &m.DisplayName, &role, &suspended, &created)
	if err == sql.ErrNoRows {
		return nil, forum.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.Role = forum.Role(role)
	m.Suspended = suspended != 0
	m.CreatedAt = parseTime(created)
	return &m, nil
}

func (s *Store) CreateSession(memberID int64, token string, expires time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, member_id, expires_at) VALUES (?, ?, ?)`,
		token, memberID, expires.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) MemberBySession(token string) (*forum.Member, error) {
	var memberID int64
	var expires string
	err := s.db.QueryRow(`SELECT member_id, expires_at FROM sessions WHERE id = ?`, token).Scan(&memberID, &expires)
	if err == sql.ErrNoRows {
		return nil, forum.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if parseTime(expires).Before(time.Now()) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, token)
		return nil, forum.ErrNotFound
	}
	m, err := s.MemberByID(memberID)
	if err != nil {
		return nil, err
	}
	if m.Suspended {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, token)
		return nil, forum.ErrNotFound
	}
	return m, nil
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, token)
	return err
}

func (s *Store) deleteSessionsForMember(memberID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE member_id = ?`, memberID)
	return err
}

func (s *Store) ListMembers() ([]forum.Member, error) {
	rows, err := s.db.Query(`SELECT id, login_name, display_name, role, suspended, created_at FROM members ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []forum.Member
	for rows.Next() {
		var m forum.Member
		var created, role string
		var suspended int
		if err := rows.Scan(&m.ID, &m.LoginName, &m.DisplayName, &role, &suspended, &created); err != nil {
			return nil, err
		}
		m.Role = forum.Role(role)
		m.Suspended = suspended != 0
		m.CreatedAt = parseTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SetMemberSuspended(actor *forum.Member, id int64, suspended bool) (*forum.Member, error) {
	target, err := s.MemberByID(id)
	if err != nil {
		return nil, err
	}
	if !forum.CanSuspend(actor, target) {
		return nil, forum.ErrCannotSuspend
	}
	v := 0
	if suspended {
		v = 1
	}
	if _, err := s.db.Exec(`UPDATE members SET suspended = ? WHERE id = ?`, v, id); err != nil {
		return nil, err
	}
	if suspended {
		if err := s.deleteSessionsForMember(id); err != nil {
			return nil, err
		}
	}
	return s.MemberByID(id)
}

func (s *Store) SetMemberRole(actor *forum.Member, id int64, role forum.Role) (*forum.Member, error) {
	if role != forum.RoleMember && role != forum.RoleOperator {
		return nil, forum.ErrCannotSetRole
	}
	target, err := s.MemberByID(id)
	if err != nil {
		return nil, err
	}
	if !forum.CanSetRole(actor, target) {
		return nil, forum.ErrCannotSetRole
	}
	if target.Role == role {
		return target, nil
	}
	if _, err := s.db.Exec(`UPDATE members SET role = ? WHERE id = ?`, string(role), id); err != nil {
		return nil, err
	}
	return s.MemberByID(id)
}

func (s *Store) SetMemberPassword(actor *forum.Member, id int64, passwordHash string) (*forum.Member, error) {
	if passwordHash == "" {
		return nil, forum.ErrPasswordEmpty
	}
	target, err := s.MemberByID(id)
	if err != nil {
		return nil, err
	}
	if !forum.CanSetPassword(actor, target) {
		return nil, forum.ErrCannotSetPassword
	}
	if _, err := s.db.Exec(`UPDATE members SET password_hash = ? WHERE id = ?`, passwordHash, id); err != nil {
		return nil, err
	}
	if err := s.deleteSessionsForMember(id); err != nil {
		return nil, err
	}
	return s.MemberByID(id)
}

func (s *Store) CreateBoard(name, description string) (*forum.Board, error) {
	name, err := forum.NormalizeBoardName(name)
	if err != nil {
		return nil, err
	}
	description, err = forum.NormalizeBoardDesc(description)
	if err != nil {
		return nil, err
	}
	var maxSort int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(sort), 0) FROM boards`).Scan(&maxSort); err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`INSERT INTO boards (name, description, sort, created_at) VALUES (?, ?, ?, ?)`,
		name, description, maxSort+1, now(),
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.BoardByID(id)
}

func (s *Store) SetBoardDisabled(actor *forum.Member, id int64, disabled bool) (*forum.Board, error) {
	if !forum.CanCreateBoard(actor) {
		return nil, forum.ErrCannotManageBoard
	}
	if _, err := s.BoardByID(id); err != nil {
		return nil, err
	}
	v := 0
	if disabled {
		v = 1
	}
	if _, err := s.db.Exec(`UPDATE boards SET disabled = ? WHERE id = ?`, v, id); err != nil {
		return nil, err
	}
	return s.BoardByID(id)
}

func (s *Store) UpdateBoard(actor *forum.Member, id int64, name, description string) (*forum.Board, error) {
	if !forum.CanCreateBoard(actor) {
		return nil, forum.ErrCannotManageBoard
	}
	if _, err := s.BoardByID(id); err != nil {
		return nil, err
	}
	name, err := forum.NormalizeBoardName(name)
	if err != nil {
		return nil, err
	}
	description, err = forum.NormalizeBoardDesc(description)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`UPDATE boards SET name = ?, description = ? WHERE id = ?`, name, description, id); err != nil {
		return nil, err
	}
	return s.BoardByID(id)
}

func (s *Store) MoveBoard(actor *forum.Member, id int64, delta int) (*forum.Board, error) {
	if !forum.CanCreateBoard(actor) {
		return nil, forum.ErrCannotManageBoard
	}
	if delta != -1 && delta != 1 {
		return s.BoardByID(id)
	}
	b, err := s.BoardByID(id)
	if err != nil {
		return nil, err
	}
	var otherID int64
	var otherSort int
	q := `SELECT id, sort FROM boards WHERE sort < ? ORDER BY sort DESC LIMIT 1`
	if delta > 0 {
		q = `SELECT id, sort FROM boards WHERE sort > ? ORDER BY sort ASC LIMIT 1`
	}
	err = s.db.QueryRow(q, b.Sort).Scan(&otherID, &otherSort)
	if err != nil {
		return b, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE boards SET sort = ? WHERE id = ?`, otherSort, b.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE boards SET sort = ? WHERE id = ?`, b.Sort, otherID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.BoardByID(id)
}

// MoveThread moves a thread to another board. Floor numbers, title, pin rank
// and timestamps are untouched.
func (s *Store) MoveThread(actor *forum.Member, threadID, boardID int64) (*forum.Thread, error) {
	if !forum.CanMoveThread(actor) {
		return nil, forum.ErrCannotMoveThread
	}
	th, err := s.ThreadByID(threadID)
	if err != nil {
		return nil, err
	}
	b, err := s.BoardByID(boardID)
	if err != nil {
		return nil, err
	}
	if th.BoardID == b.ID {
		return th, nil
	}
	if _, err := s.db.Exec(`UPDATE threads SET board_id = ? WHERE id = ?`, b.ID, th.ID); err != nil {
		return nil, err
	}
	return s.ThreadByID(threadID)
}

func (s *Store) ListBoards() ([]forum.Board, error) {
	rows, err := s.db.Query(`SELECT id, name, description, sort, disabled, created_at FROM boards ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []forum.Board
	for rows.Next() {
		var b forum.Board
		var created string
		var disabled int
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.Sort, &disabled, &created); err != nil {
			return nil, err
		}
		b.Disabled = disabled != 0
		b.CreatedAt = parseTime(created)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) BoardByID(id int64) (*forum.Board, error) {
	var b forum.Board
	var created string
	var disabled int
	err := s.db.QueryRow(
		`SELECT id, name, description, sort, disabled, created_at FROM boards WHERE id = ?`,
		id,
	).Scan(&b.ID, &b.Name, &b.Description, &b.Sort, &disabled, &created)
	if err == sql.ErrNoRows {
		return nil, forum.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.Disabled = disabled != 0
	b.CreatedAt = parseTime(created)
	return &b, nil
}

func (s *Store) CreateThread(boardID, authorID int64, title, body string) (*forum.Thread, *forum.Post, error) {
	title, err := forum.NormalizeTitle(title)
	if err != nil {
		return nil, nil, err
	}
	body, err = forum.NormalizeBody(body)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.BoardByID(boardID); err != nil {
		return nil, nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	ts := now()
	res, err := tx.Exec(
		`INSERT INTO threads (board_id, title, author_id, created_at, last_post_at) VALUES (?, ?, ?, ?, ?)`,
		boardID, title, authorID, ts, ts,
	)
	if err != nil {
		return nil, nil, err
	}
	threadID, err := res.LastInsertId()
	if err != nil {
		return nil, nil, err
	}
	pres, err := tx.Exec(
		`INSERT INTO posts (thread_id, author_id, floor, body_markdown, created_at) VALUES (?, ?, ?, ?, ?)`,
		threadID, authorID, forum.FirstFloor, body, ts,
	)
	if err != nil {
		return nil, nil, err
	}
	postID, err := pres.LastInsertId()
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	th := &forum.Thread{
		ID:         threadID,
		BoardID:    boardID,
		Title:      title,
		AuthorID:   authorID,
		CreatedAt:  parseTime(ts),
		LastPostAt: parseTime(ts),
	}
	p := &forum.Post{
		ID:           postID,
		ThreadID:     threadID,
		AuthorID:     authorID,
		Floor:        forum.FirstFloor,
		BodyMarkdown: body,
		CreatedAt:    parseTime(ts),
	}
	return th, p, nil
}

func (s *Store) ListThreads(boardID int64, viewer *forum.Member) ([]forum.ThreadView, error) {
	q := `
		SELECT t.id, t.board_id, t.title, t.author_id, t.created_at, t.last_post_at, t.title_edited_at, t.pin_rank,
		       m.login_name, m.display_name,
		       COALESCE((SELECT p.hidden FROM posts p WHERE p.thread_id = t.id AND p.floor = 1), 0)
		FROM threads t
		JOIN members m ON m.id = t.author_id
		WHERE t.board_id = ?
		ORDER BY CASE WHEN t.pin_rank > 0 THEN 0 ELSE 1 END,
		         CASE WHEN t.pin_rank > 0 THEN t.pin_rank ELSE 0 END,
		         t.last_post_at DESC, t.id DESC
	`
	rows, err := s.db.Query(q, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []forum.ThreadView
	for rows.Next() {
		var v forum.ThreadView
		var created, last string
		var titleEdited sql.NullString
		var hidden int
		if err := rows.Scan(&v.ID, &v.BoardID, &v.Title, &v.AuthorID, &created, &last, &titleEdited, &v.PinRank, &v.AuthorLoginName, &v.AuthorDisplayName, &hidden); err != nil {
			return nil, err
		}
		v.CreatedAt = parseTime(created)
		v.LastPostAt = parseTime(last)
		if titleEdited.Valid && titleEdited.String != "" {
			t := parseTime(titleEdited.String)
			v.TitleEditedAt = &t
		}
		v.FirstHidden = hidden != 0
		if v.FirstHidden && !forum.CanHidePost(viewer) {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ThreadByID(id int64) (*forum.Thread, error) {
	var th forum.Thread
	var created, last string
	var titleEdited sql.NullString
	err := s.db.QueryRow(
		`SELECT id, board_id, title, author_id, created_at, last_post_at, title_edited_at, pin_rank FROM threads WHERE id = ?`,
		id,
	).Scan(&th.ID, &th.BoardID, &th.Title, &th.AuthorID, &created, &last, &titleEdited, &th.PinRank)
	if err == sql.ErrNoRows {
		return nil, forum.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	th.CreatedAt = parseTime(created)
	th.LastPostAt = parseTime(last)
	if titleEdited.Valid && titleEdited.String != "" {
		t := parseTime(titleEdited.String)
		th.TitleEditedAt = &t
	}
	return &th, nil
}

func (s *Store) CreatePost(threadID, authorID int64, body string) (*forum.Post, error) {
	body, err := forum.NormalizeBody(body)
	if err != nil {
		return nil, err
	}
	posts, err := s.ListPosts(threadID)
	if err != nil {
		return nil, err
	}
	if forum.ThreadHiddenFromMembers(posts) {
		actor, aerr := s.MemberByID(authorID)
		if aerr != nil {
			return nil, aerr
		}
		if !forum.CanHidePost(actor) {
			return nil, forum.ErrCannotReply
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var lastFloor int
	err = tx.QueryRow(`SELECT COALESCE(MAX(floor), 0) FROM posts WHERE thread_id = ?`, threadID).Scan(&lastFloor)
	if err != nil {
		return nil, err
	}
	if lastFloor == 0 {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM threads WHERE id = ?`, threadID).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, forum.ErrNotFound
		}
	}
	floor := forum.NextFloor(lastFloor)
	ts := now()
	res, err := tx.Exec(
		`INSERT INTO posts (thread_id, author_id, floor, body_markdown, created_at) VALUES (?, ?, ?, ?, ?)`,
		threadID, authorID, floor, body, ts,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE threads SET last_post_at = ? WHERE id = ?`, ts, threadID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &forum.Post{
		ID:           id,
		ThreadID:     threadID,
		AuthorID:     authorID,
		Floor:        floor,
		BodyMarkdown: body,
		CreatedAt:    parseTime(ts),
	}, nil
}

func (s *Store) ListPosts(threadID int64) ([]forum.PostView, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.thread_id, p.author_id, p.floor, p.body_markdown, p.created_at, p.edited_at, p.hidden,
		       m.login_name, m.display_name, m.role
		FROM posts p
		JOIN members m ON m.id = p.author_id
		WHERE p.thread_id = ?
		ORDER BY p.floor ASC
	`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []forum.PostView
	for rows.Next() {
		var v forum.PostView
		var created, role string
		var edited sql.NullString
		var hidden int
		if err := rows.Scan(&v.ID, &v.ThreadID, &v.AuthorID, &v.Floor, &v.BodyMarkdown, &created, &edited, &hidden, &v.AuthorLoginName, &v.AuthorDisplayName, &role); err != nil {
			return nil, err
		}
		v.CreatedAt = parseTime(created)
		if edited.Valid && edited.String != "" {
			t := parseTime(edited.String)
			v.EditedAt = &t
		}
		v.Hidden = hidden != 0
		v.AuthorRole = forum.Role(role)
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanInvite(scanner interface {
	Scan(dest ...any) error
}) (forum.InviteCode, error) {
	var c forum.InviteCode
	var issued string
	var revoked int
	var usedLogin, usedAt sql.NullString
	if err := scanner.Scan(&c.ID, &c.Code, &c.IssuedByID, &c.IssuedByLogin, &issued, &revoked, &usedLogin, &usedAt); err != nil {
		return c, err
	}
	c.IssuedAt = parseTime(issued)
	c.Revoked = revoked != 0
	if usedLogin.Valid {
		c.UsedByLogin = usedLogin.String
	}
	if usedAt.Valid {
		t := parseTime(usedAt.String)
		c.UsedAt = &t
	}
	return c, nil
}

const inviteSelect = `SELECT i.id, i.code, i.issued_by, m.login_name, i.issued_at, i.revoked, i.used_by_login, i.used_at
FROM invite_codes i JOIN members m ON m.id = i.issued_by`

func (s *Store) IssueInvite(issuer *forum.Member, code string) (*forum.InviteCode, error) {
	if !forum.CanIssueInvite(issuer) {
		return nil, forum.ErrCannotIssueInvite
	}
	if code == "" {
		return nil, forum.ErrInviteInvalid
	}
	ts := now()
	res, err := s.db.Exec(
		`INSERT INTO invite_codes (code, issued_by, issued_at, revoked) VALUES (?, ?, ?, 0)`,
		code, issuer.ID, ts,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, forum.ErrInviteInvalid
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.InviteByID(id)
}

func (s *Store) InviteByCode(code string) (*forum.InviteCode, error) {
	c, err := scanInvite(s.db.QueryRow(inviteSelect+` WHERE i.code = ?`, code))
	if err == sql.ErrNoRows {
		return nil, forum.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) InviteByID(id int64) (*forum.InviteCode, error) {
	c, err := scanInvite(s.db.QueryRow(inviteSelect+` WHERE i.id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, forum.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListInvites() ([]forum.InviteCode, error) {
	rows, err := s.db.Query(inviteSelect + ` ORDER BY i.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []forum.InviteCode
	for rows.Next() {
		c, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) RevokeInvite(actor *forum.Member, id int64) error {
	if !forum.CanIssueInvite(actor) {
		return forum.ErrCannotIssueInvite
	}
	c, err := s.InviteByID(id)
	if err != nil {
		return err
	}
	if err := forum.InviteUsable(c); err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE invite_codes SET revoked = 1 WHERE id = ? AND revoked = 0 AND used_at IS NULL`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return forum.ErrInviteUsed
	}
	return nil
}

func (s *Store) Register(code, loginName, displayName, passwordHash string) (*forum.Member, error) {
	if !forum.ValidLoginName(loginName) {
		return nil, forum.ErrInvalidLoginName
	}
	displayName, err := forum.NormalizeDisplayName(displayName)
	if err != nil {
		return nil, err
	}
	if passwordHash == "" {
		return nil, forum.ErrPasswordEmpty
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var invID int64
	var revoked int
	var usedLogin sql.NullString
	err = tx.QueryRow(
		`SELECT id, revoked, used_by_login FROM invite_codes WHERE code = ?`,
		code,
	).Scan(&invID, &revoked, &usedLogin)
	if err == sql.ErrNoRows {
		return nil, forum.ErrInviteInvalid
	}
	if err != nil {
		return nil, err
	}
	inv := &forum.InviteCode{ID: invID, Revoked: revoked != 0}
	if usedLogin.Valid {
		inv.UsedByLogin = usedLogin.String
	}
	if err := forum.InviteUsable(inv); err != nil {
		return nil, err
	}

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM members WHERE login_name = ?`, loginName).Scan(&exists); err != nil {
		return nil, err
	}
	if exists > 0 {
		return nil, forum.ErrLoginNameTaken
	}

	ts := now()
	res, err := tx.Exec(
		`INSERT INTO members (login_name, display_name, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		loginName, displayName, passwordHash, string(forum.RoleMember), ts,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	r, err := tx.Exec(
		`UPDATE invite_codes SET used_by_login = ?, used_at = ? WHERE id = ? AND revoked = 0 AND used_at IS NULL`,
		loginName, ts, invID,
	)
	if err != nil {
		return nil, err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, forum.ErrInviteUsed
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.MemberByID(id)
}
