package store

import (
	"database/sql"

	"go-forum/internal/forum"
)

func (s *Store) PostByID(id int64) (*forum.Post, error) {
	var p forum.Post
	var created string
	var edited sql.NullString
	var hidden int
	err := s.db.QueryRow(
		`SELECT id, thread_id, author_id, floor, body_markdown, created_at, edited_at, hidden FROM posts WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.ThreadID, &p.AuthorID, &p.Floor, &p.BodyMarkdown, &created, &edited, &hidden)
	if err == sql.ErrNoRows {
		return nil, forum.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.CreatedAt = parseTime(created)
	if edited.Valid && edited.String != "" {
		t := parseTime(edited.String)
		p.EditedAt = &t
	}
	p.Hidden = hidden != 0
	return &p, nil
}

func (s *Store) UpdatePost(actor *forum.Member, postID int64, newBody string) (*forum.Post, error) {
	newBody, err := forum.NormalizeBody(newBody)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var p forum.Post
	var created string
	var edited sql.NullString
	var hidden int
	err = tx.QueryRow(
		`SELECT id, thread_id, author_id, floor, body_markdown, created_at, edited_at, hidden FROM posts WHERE id = ?`,
		postID,
	).Scan(&p.ID, &p.ThreadID, &p.AuthorID, &p.Floor, &p.BodyMarkdown, &created, &edited, &hidden)
	if err == sql.ErrNoRows {
		return nil, forum.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.CreatedAt = parseTime(created)
	if edited.Valid && edited.String != "" {
		t := parseTime(edited.String)
		p.EditedAt = &t
	}
	p.Hidden = hidden != 0
	if !forum.CanEditPost(actor, &p) {
		return nil, forum.ErrCannotEditPost
	}
	if newBody == p.BodyMarkdown {
		return &p, nil
	}
	ts := now()
	if _, err := tx.Exec(
		`INSERT INTO post_edits (post_id, body_markdown, edited_at) VALUES (?, ?, ?)`,
		p.ID, p.BodyMarkdown, ts,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`UPDATE posts SET body_markdown = ?, edited_at = ? WHERE id = ?`,
		newBody, ts, p.ID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	t := parseTime(ts)
	p.BodyMarkdown = newBody
	p.EditedAt = &t
	return &p, nil
}

func (s *Store) ListEdits(postID int64) ([]forum.Edit, error) {
	if _, err := s.PostByID(postID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, post_id, body_markdown, edited_at FROM post_edits WHERE post_id = ? ORDER BY edited_at DESC, id DESC`,
		postID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []forum.Edit
	for rows.Next() {
		var e forum.Edit
		var edited string
		if err := rows.Scan(&e.ID, &e.PostID, &e.BodyMarkdown, &edited); err != nil {
			return nil, err
		}
		e.EditedAt = parseTime(edited)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) SetPostHidden(actor *forum.Member, postID int64, hidden bool) (*forum.Post, error) {
	if !forum.CanHidePost(actor) {
		return nil, forum.ErrCannotHidePost
	}
	p, err := s.PostByID(postID)
	if err != nil {
		return nil, err
	}
	v := 0
	if hidden {
		v = 1
	}
	if _, err := s.db.Exec(`UPDATE posts SET hidden = ? WHERE id = ?`, v, postID); err != nil {
		return nil, err
	}
	p.Hidden = hidden
	return p, nil
}

func (s *Store) UpdateThreadTitle(actor *forum.Member, threadID int64, newTitle string) (*forum.Thread, error) {
	newTitle, err := forum.NormalizeTitle(newTitle)
	if err != nil {
		return nil, err
	}
	th, err := s.ThreadByID(threadID)
	if err != nil {
		return nil, err
	}
	posts, err := s.ListPosts(threadID)
	if err != nil {
		return nil, err
	}
	if !forum.CanEditTitle(actor, th, forum.ThreadHiddenFromMembers(posts)) {
		return nil, forum.ErrCannotEditTitle
	}
	if newTitle == th.Title {
		return th, nil
	}
	ts := now()
	if _, err := s.db.Exec(`UPDATE threads SET title = ?, title_edited_at = ? WHERE id = ?`, newTitle, ts, threadID); err != nil {
		return nil, err
	}
	t := parseTime(ts)
	th.Title = newTitle
	th.TitleEditedAt = &t
	return th, nil
}

func (s *Store) PinThread(actor *forum.Member, threadID int64) (*forum.Thread, error) {
	if !forum.CanPin(actor) {
		return nil, forum.ErrCannotPin
	}
	th, err := s.ThreadByID(threadID)
	if err != nil {
		return nil, err
	}
	if th.Pinned() {
		return th, nil
	}
	var maxRank int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(pin_rank), 0) FROM threads WHERE board_id = ?`, th.BoardID).Scan(&maxRank); err != nil {
		return nil, err
	}
	rank := maxRank + 1
	if _, err := s.db.Exec(`UPDATE threads SET pin_rank = ? WHERE id = ?`, rank, threadID); err != nil {
		return nil, err
	}
	th.PinRank = rank
	return th, nil
}

func (s *Store) UnpinThread(actor *forum.Member, threadID int64) (*forum.Thread, error) {
	if !forum.CanPin(actor) {
		return nil, forum.ErrCannotPin
	}
	th, err := s.ThreadByID(threadID)
	if err != nil {
		return nil, err
	}
	if !th.Pinned() {
		return th, nil
	}
	if _, err := s.db.Exec(`UPDATE threads SET pin_rank = 0 WHERE id = ?`, threadID); err != nil {
		return nil, err
	}
	th.PinRank = 0
	return th, nil
}

func (s *Store) MovePinned(actor *forum.Member, threadID int64, delta int) (*forum.Thread, error) {
	if !forum.CanPin(actor) {
		return nil, forum.ErrCannotPin
	}
	if delta != -1 && delta != 1 {
		return s.ThreadByID(threadID)
	}
	th, err := s.ThreadByID(threadID)
	if err != nil {
		return nil, err
	}
	if !th.Pinned() {
		return th, nil
	}
	var otherID int64
	var otherRank int
	q := `SELECT id, pin_rank FROM threads WHERE board_id = ? AND pin_rank > 0 AND pin_rank < ? ORDER BY pin_rank DESC LIMIT 1`
	if delta > 0 {
		q = `SELECT id, pin_rank FROM threads WHERE board_id = ? AND pin_rank > 0 AND pin_rank > ? ORDER BY pin_rank ASC LIMIT 1`
	}
	arg := th.PinRank
	err = s.db.QueryRow(q, th.BoardID, arg).Scan(&otherID, &otherRank)
	if err != nil {
		return th, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE threads SET pin_rank = ? WHERE id = ?`, otherRank, th.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE threads SET pin_rank = ? WHERE id = ?`, th.PinRank, otherID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	th.PinRank = otherRank
	return th, nil
}
