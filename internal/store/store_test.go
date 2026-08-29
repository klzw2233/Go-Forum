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
