package store

import (
	"path/filepath"
	"testing"
	"time"

	"go-forum/internal/forum"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func founder(t *testing.T, s *Store) *forum.Member {
	t.Helper()
	h, err := forum.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.EnsureFounder("jimmy", "Jimmy", h)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestEnsureFounderDoesNotOverwritePassword(t *testing.T) {
	s := testStore(t)
	h1, err := forum.HashPassword("first")
	if err != nil {
		t.Fatal(err)
	}
	m1, err := s.EnsureFounder("jimmy", "Jimmy", h1)
	if err != nil {
		t.Fatal(err)
	}
	if m1.Role != forum.RoleFounder || m1.LoginName != "jimmy" {
		t.Fatalf("founder: %+v", m1)
	}

	h2, err := forum.HashPassword("second")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := s.EnsureFounder("jimmy", "Other", h2)
	if err != nil {
		t.Fatal(err)
	}
	if m2.ID != m1.ID {
		t.Fatalf("id changed")
	}
	_, hash, err := s.MemberByLogin("jimmy")
	if err != nil {
		t.Fatal(err)
	}
	if !forum.CheckPassword(hash, "first") {
		t.Fatal("password was overwritten")
	}
	if forum.CheckPassword(hash, "second") {
		t.Fatal("new password should not work")
	}
	if m2.DisplayName != "Jimmy" {
		t.Fatalf("display name overwritten: %q", m2.DisplayName)
	}
}

func TestCreateThreadAndReplyFloors(t *testing.T) {
	s := testStore(t)
	h, err := forum.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.EnsureFounder("jimmy", "Jimmy", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "闲聊")
	if err != nil {
		t.Fatal(err)
	}
	th, p1, err := s.CreateThread(b.ID, m.ID, "你好", "一楼正文")
	if err != nil {
		t.Fatal(err)
	}
	if p1.Floor != 1 {
		t.Fatalf("first floor = %d", p1.Floor)
	}
	p2, err := s.CreatePost(th.ID, m.ID, "二楼")
	if err != nil {
		t.Fatal(err)
	}
	if p2.Floor != 2 {
		t.Fatalf("second floor = %d", p2.Floor)
	}
	p3, err := s.CreatePost(th.ID, m.ID, "三楼")
	if err != nil {
		t.Fatal(err)
	}
	if p3.Floor != 3 {
		t.Fatalf("third floor = %d", p3.Floor)
	}
	posts, err := s.ListPosts(th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 3 {
		t.Fatalf("len posts = %d", len(posts))
	}
	for i, p := range posts {
		if p.Floor != i+1 {
			t.Fatalf("posts[%d].Floor = %d", i, p.Floor)
		}
	}
	if posts[0].BodyMarkdown != "一楼正文" || posts[1].BodyMarkdown != "二楼" {
		t.Fatalf("bodies: %+v", posts)
	}

	if _, _, err := s.CreateThread(b.ID, m.ID, "", "x"); err != forum.ErrTitleEmpty {
		t.Fatalf("empty title: %v", err)
	}
	if _, _, err := s.CreateThread(b.ID, m.ID, "t", ""); err != forum.ErrBodyEmpty {
		t.Fatalf("empty body: %v", err)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	s := testStore(t)
	h, _ := forum.HashPassword("secret")
	m, err := s.EnsureFounder("jimmy", "Jimmy", h)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(m.ID, "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.MemberBySession("tok")
	if err != nil || got.ID != m.ID {
		t.Fatalf("got %+v err %v", got, err)
	}
	if err := s.DeleteSession("tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MemberBySession("tok"); err != forum.ErrNotFound {
		t.Fatalf("deleted session: %v", err)
	}
}

func TestInviteIssueRegisterRevoke(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)

	if _, err := s.IssueInvite(&forum.Member{ID: f.ID, Role: forum.RoleMember}, "abc"); err != forum.ErrCannotIssueInvite {
		t.Fatalf("member issue: %v", err)
	}

	inv, err := s.IssueInvite(f, "codeAAAA1111")
	if err != nil {
		t.Fatal(err)
	}
	if inv.IssuedByLogin != "jimmy" || inv.Status() != "未使用" {
		t.Fatalf("%+v", inv)
	}

	h, err := forum.HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Register("codeAAAA1111", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	if m.Role != forum.RoleMember || m.LoginName != "wang" {
		t.Fatalf("%+v", m)
	}
	used, err := s.InviteByCode("codeAAAA1111")
	if err != nil {
		t.Fatal(err)
	}
	if used.UsedByLogin != "wang" || used.Status() != "已使用" {
		t.Fatalf("%+v", used)
	}

	if _, err := s.Register("codeAAAA1111", "li", "李", h); err != forum.ErrInviteUsed {
		t.Fatalf("reuse: %v", err)
	}
	if err := s.RevokeInvite(f, inv.ID); err != forum.ErrInviteUsed {
		t.Fatalf("revoke used: %v", err)
	}

	inv2, err := s.IssueInvite(f, "codeBBBB2222")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register("codeBBBB2222", "wang", "占用", h); err != forum.ErrLoginNameTaken {
		t.Fatalf("taken: %v", err)
	}
	still, err := s.InviteByID(inv2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.Status() != "未使用" {
		t.Fatalf("code consumed on name clash: %+v", still)
	}

	if err := s.RevokeInvite(f, inv2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register("codeBBBB2222", "zhao", "赵", h); err != forum.ErrInviteRevoked {
		t.Fatalf("revoked register: %v", err)
	}

	_, hash, err := s.MemberByLogin("jimmy")
	if err != nil {
		t.Fatal(err)
	}
	if !forum.CheckPassword(hash, "secret") {
		t.Fatal("founder password changed")
	}
}

func TestUpdatePostKeepsLastPostAtAndHistory(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, p1, err := s.CreateThread(b.ID, f.ID, "题", "旧文")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ThreadByID(th.ID)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdatePost(f, p1.ID, "新文")
	if err != nil {
		t.Fatal(err)
	}
	if got.BodyMarkdown != "新文" || got.EditedAt == nil {
		t.Fatalf("%+v", got)
	}
	after, err := s.ThreadByID(th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.LastPostAt.Equal(before.LastPostAt) {
		t.Fatalf("last_post_at moved %v -> %v", before.LastPostAt, after.LastPostAt)
	}
	edits, err := s.ListEdits(p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].BodyMarkdown != "旧文" {
		t.Fatalf("%+v", edits)
	}

	if _, err := s.UpdatePost(f, p1.ID, "更新"); err != nil {
		t.Fatal(err)
	}
	edits, err = s.ListEdits(p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 2 {
		t.Fatalf("want 2 edits, got %d", len(edits))
	}

	if _, err := s.UpdatePost(f, p1.ID, ""); err != forum.ErrBodyEmpty {
		t.Fatalf("empty: %v", err)
	}
	other := &forum.Member{ID: f.ID + 99, Role: forum.RoleMember}
	if _, err := s.UpdatePost(other, p1.ID, "偷改"); err != forum.ErrCannotEditPost {
		t.Fatalf("other: %v", err)
	}
}
