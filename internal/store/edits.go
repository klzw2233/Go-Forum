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
