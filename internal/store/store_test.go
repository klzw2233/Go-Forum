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

func TestHidePostAndThreadList(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := s.IssueInvite(f, "hidecodeaaaa")
	if err != nil {
		t.Fatal(err)
	}
	_ = inv
	mem, err := s.Register("hidecodeaaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, _, err := s.CreateThread(b.ID, mem.ID, "可见主题", "一楼")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreatePost(th.ID, mem.ID, "二楼秘密")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPostHidden(mem, p2.ID, true); err != forum.ErrCannotHidePost {
		t.Fatalf("member hide: %v", err)
	}
	got, err := s.SetPostHidden(f, p2.ID, true)
	if err != nil || !got.Hidden {
		t.Fatalf("hide 2: %+v %v", got, err)
	}
	posts, err := s.ListPosts(th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !posts[1].Hidden || posts[1].BodyMarkdown != "二楼秘密" {
		t.Fatalf("%+v", posts[1])
	}
	if _, err := s.UpdatePost(mem, p2.ID, "改藏着的"); err != forum.ErrCannotEditPost {
		t.Fatalf("edit hidden: %v", err)
	}

	th2, _, err := s.CreateThread(b.ID, mem.ID, "藏起来的主题", "不该给会员看")
	if err != nil {
		t.Fatal(err)
	}
	posts2, err := s.ListPosts(th2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPostHidden(f, posts2[0].ID, true); err != nil {
		t.Fatal(err)
	}
	listed, err := s.ListThreads(b.ID, mem)
	if err != nil {
		t.Fatal(err)
	}
	for _, tview := range listed {
		if tview.Title == "藏起来的主题" {
			t.Fatal("member saw hidden thread")
		}
	}
	staffList, err := s.ListThreads(b.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, tview := range staffList {
		if tview.Title == "藏起来的主题" {
			saw = true
			if !tview.FirstHidden {
				t.Fatal("staff list missing FirstHidden")
			}
		}
	}
	if !saw {
		t.Fatal("founder must see hidden thread")
	}
	if _, err := s.CreatePost(th2.ID, mem.ID, "会员回"); err != forum.ErrCannotReply {
		t.Fatalf("member reply hidden thread: %v", err)
	}
	if _, err := s.CreatePost(th2.ID, f.ID, "创始人回"); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateThreadTitle(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "titlecodeaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("titlecodeaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, _, err := s.CreateThread(b.ID, mem.ID, "旧标题", "一楼")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ThreadByID(th.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateThreadTitle(mem, th.ID, "新标题")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "新标题" || got.TitleEditedAt == nil {
		t.Fatalf("%+v", got)
	}
	after, err := s.ThreadByID(th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.LastPostAt.Equal(before.LastPostAt) {
		t.Fatal("last_post_at moved")
	}
	if _, err := s.UpdateThreadTitle(mem, th.ID, ""); err != forum.ErrTitleEmpty {
		t.Fatalf("empty: %v", err)
	}
	other := &forum.Member{ID: mem.ID + 99, Role: forum.RoleMember}
	if _, err := s.UpdateThreadTitle(other, th.ID, "偷改"); err != forum.ErrCannotEditTitle {
		t.Fatalf("other: %v", err)
	}
	posts, err := s.ListPosts(th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPostHidden(f, posts[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateThreadTitle(mem, th.ID, "作者再改"); err != forum.ErrCannotEditTitle {
		t.Fatalf("author after hide: %v", err)
	}
	if _, err := s.UpdateThreadTitle(f, th.ID, "创始人改"); err != nil {
		t.Fatal(err)
	}
}

func TestPinOrderAndMove(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "pincodeaaaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("pincodeaaaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := s.CreateThread(b.ID, f.ID, "A", "a")
	if err != nil {
		t.Fatal(err)
	}
	bb, _, err := s.CreateThread(b.ID, f.ID, "B", "b")
	if err != nil {
		t.Fatal(err)
	}
	cc, _, err := s.CreateThread(b.ID, f.ID, "C", "c")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PinThread(mem, bb.ID); err != forum.ErrCannotPin {
		t.Fatalf("member pin: %v", err)
	}
	if _, err := s.PinThread(f, bb.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PinThread(f, cc.ID); err != nil {
		t.Fatal(err)
	}
	listed, err := s.ListThreads(b.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) < 3 || listed[0].Title != "B" || listed[1].Title != "C" || listed[2].Title != "A" {
		t.Fatalf("order after pin B then C: %+v", titles(listed))
	}
	if _, err := s.MovePinned(f, cc.ID, -1); err != nil {
		t.Fatal(err)
	}
	listed, err = s.ListThreads(b.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Title != "C" || listed[1].Title != "B" {
		t.Fatalf("after move C up: %+v", titles(listed))
	}
	if _, err := s.UnpinThread(f, bb.ID); err != nil {
		t.Fatal(err)
	}
	listed, err = s.ListThreads(b.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Title != "C" || listed[1].Title != "B" || listed[2].Title != "A" {
		t.Fatalf("after unpin B: %+v", titles(listed))
	}
	_ = a
}

func TestSetBoardDisabled(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "disablecode1"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("disablecode1", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBoardDisabled(mem, b.ID, true); err != forum.ErrCannotManageBoard {
		t.Fatalf("member disable: %v", err)
	}
	got, err := s.SetBoardDisabled(f, b.ID, true)
	if err != nil || !got.Disabled {
		t.Fatalf("disable: %+v err=%v", got, err)
	}
	// Store keeps the board readable; member-side hiding is the handler's job.
	again, err := s.BoardByID(b.ID)
	if err != nil || !again.Disabled {
		t.Fatalf("BoardByID after disable: %+v err=%v", again, err)
	}
	list, err := s.ListBoards()
	if err != nil || len(list) != 1 || !list[0].Disabled {
		t.Fatalf("ListBoards after disable: %+v err=%v", list, err)
	}
	back, err := s.SetBoardDisabled(f, b.ID, false)
	if err != nil || back.Disabled {
		t.Fatalf("enable: %+v err=%v", back, err)
	}
}

func titles(in []forum.ThreadView) []string {
	out := make([]string, len(in))
	for i, t := range in {
		out[i] = t.Title
	}
	return out
}
