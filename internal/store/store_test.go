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

func TestMoveThread(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "movecodeaaaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("movecodeaaaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b1, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	b2, err := s.CreateBoard("技术", "")
	if err != nil {
		t.Fatal(err)
	}
	th, p1, err := s.CreateThread(b1.ID, mem.ID, "挪我", "一楼")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePost(th.ID, mem.ID, "二楼"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PinThread(f, th.ID); err != nil {
		t.Fatal(err)
	}
	before, err := s.ThreadByID(th.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.MoveThread(mem, th.ID, b2.ID); err != forum.ErrCannotMoveThread {
		t.Fatalf("member move: %v", err)
	}
	if _, err := s.MoveThread(f, th.ID, b2.ID); err != nil {
		t.Fatal(err)
	}

	// Appears in new board, gone from old; floors and pin kept.
	oldList, err := s.ListThreads(b1.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, tv := range oldList {
		if tv.ID == th.ID {
			t.Fatal("thread still in old board")
		}
	}
	newList, err := s.ListThreads(b2.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(newList) != 1 || newList[0].ID != th.ID || !newList[0].Pinned() {
		t.Fatalf("thread not in new board or lost pin: %+v", newList)
	}
	posts, err := s.ListPosts(th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 || posts[0].Floor != 1 || posts[1].Floor != 2 {
		t.Fatalf("floors changed: %+v", posts)
	}
	after, err := s.ThreadByID(th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Title != "挪我" || !after.LastPostAt.Equal(before.LastPostAt) {
		t.Fatalf("title or last_post_at changed: %+v", after)
	}
	_ = p1

	// Unknown board / thread.
	if _, err := s.MoveThread(f, th.ID, b2.ID+99); err != forum.ErrNotFound {
		t.Fatalf("unknown board: %v", err)
	}
	if _, err := s.MoveThread(f, th.ID+99, b1.ID); err != forum.ErrNotFound {
		t.Fatalf("unknown thread: %v", err)
	}
	// Moving to the same board is a noop.
	same, err := s.MoveThread(f, th.ID, b2.ID)
	if err != nil || same.BoardID != b2.ID {
		t.Fatalf("same board noop: %+v err=%v", same, err)
	}
}

func titles(in []forum.ThreadView) []string {
	out := make([]string, len(in))
	for i, t := range in {
		out[i] = t.Title
	}
	return out
}

func TestSetMemberSuspended(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "suspendcode01"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("suspendcode01", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, _, err := s.CreateThread(b.ID, mem.ID, "旧帖", "还在")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(mem.ID, "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := s.SetMemberSuspended(mem, mem.ID, true); err != forum.ErrCannotSuspend {
		t.Fatalf("self: %v", err)
	}
	if _, err := s.SetMemberSuspended(f, f.ID, true); err != forum.ErrCannotSuspend {
		t.Fatalf("founder self: %v", err)
	}
	if _, err := s.SetMemberSuspended(mem, f.ID, true); err != forum.ErrCannotSuspend {
		t.Fatalf("member vs founder: %v", err)
	}
	if _, err := s.SetMemberSuspended(f, mem.ID+99, true); err != forum.ErrNotFound {
		t.Fatalf("unknown: %v", err)
	}

	got, err := s.SetMemberSuspended(f, mem.ID, true)
	if err != nil || !got.Suspended {
		t.Fatalf("suspend: %+v err=%v", got, err)
	}
	if _, err := s.MemberBySession("tok"); err != forum.ErrNotFound {
		t.Fatalf("session after suspend: %v", err)
	}
	again, err := s.MemberByID(mem.ID)
	if err != nil || !again.Suspended || again.LoginName != "wang" {
		t.Fatalf("MemberByID: %+v err=%v", again, err)
	}
	if _, err := s.IssueInvite(f, "suspendcode02"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register("suspendcode02", "wang", "另一个王", h); err != forum.ErrLoginNameTaken {
		t.Fatalf("login name released: %v", err)
	}
	posts, err := s.ListPosts(th.ID)
	if err != nil || len(posts) != 1 || posts[0].BodyMarkdown != "还在" || posts[0].Hidden {
		t.Fatalf("old post changed: %+v err=%v", posts, err)
	}

	back, err := s.SetMemberSuspended(f, mem.ID, false)
	if err != nil || back.Suspended {
		t.Fatalf("restore: %+v err=%v", back, err)
	}
	if err := s.CreateSession(mem.ID, "tok2", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	live, err := s.MemberBySession("tok2")
	if err != nil || live.ID != mem.ID || live.Suspended {
		t.Fatalf("session after restore: %+v err=%v", live, err)
	}
}

func TestSetMemberRoleAndPassword(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("oldpass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "rolecodeaaaaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("rolecodeaaaaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(mem.ID, "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := s.SetMemberRole(mem, mem.ID, forum.RoleOperator); err != forum.ErrCannotSetRole {
		t.Fatalf("member promote self: %v", err)
	}
	if _, err := s.SetMemberRole(f, f.ID, forum.RoleOperator); err != forum.ErrCannotSetRole {
		t.Fatalf("founder demote self: %v", err)
	}
	if _, err := s.SetMemberRole(f, mem.ID, forum.RoleFounder); err != forum.ErrCannotSetRole {
		t.Fatalf("promote to founder: %v", err)
	}

	got, err := s.SetMemberRole(f, mem.ID, forum.RoleOperator)
	if err != nil || got.Role != forum.RoleOperator {
		t.Fatalf("promote: %+v err=%v", got, err)
	}
	same, err := s.SetMemberRole(f, mem.ID, forum.RoleOperator)
	if err != nil || same.Role != forum.RoleOperator {
		t.Fatalf("promote noop: %+v err=%v", same, err)
	}
	if _, err := s.SetMemberPassword(got, f.ID, "x"); err != forum.ErrCannotSetPassword {
		t.Fatalf("operator set founder password: %v", err)
	}
	if _, err := s.SetMemberRole(got, mem.ID, forum.RoleMember); err != forum.ErrCannotSetRole {
		t.Fatalf("operator demote: %v", err)
	}

	nh, err := forum.HashPassword("newpass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetMemberPassword(f, mem.ID, nh); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MemberBySession("tok"); err != forum.ErrNotFound {
		t.Fatalf("session after password: %v", err)
	}
	_, hash, err := s.MemberByLogin("wang")
	if err != nil {
		t.Fatal(err)
	}
	if !forum.CheckPassword(hash, "newpass") || forum.CheckPassword(hash, "oldpass") {
		t.Fatal("password not replaced")
	}

	back, err := s.SetMemberRole(f, mem.ID, forum.RoleMember)
	if err != nil || back.Role != forum.RoleMember {
		t.Fatalf("demote: %+v err=%v", back, err)
	}
}

func TestUpdateAndMoveBoard(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "editboardcode"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("editboardcode", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateBoard("灌水", "闲聊")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("技术", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateBoard(mem, a.ID, "新名", "新说明"); err != forum.ErrCannotManageBoard {
		t.Fatalf("member update: %v", err)
	}
	if _, err := s.MoveBoard(mem, a.ID, 1); err != forum.ErrCannotManageBoard {
		t.Fatalf("member move: %v", err)
	}
	if _, err := s.UpdateBoard(f, a.ID, "   ", ""); err != forum.ErrBoardNameEmpty {
		t.Fatalf("empty name: %v", err)
	}
	got, err := s.UpdateBoard(f, a.ID, " 水区 ", " 随便聊 ")
	if err != nil || got.Name != "水区" || got.Description != "随便聊" {
		t.Fatalf("update: %+v err=%v", got, err)
	}

	list, err := s.ListBoards()
	if err != nil || len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("initial order: %+v err=%v", list, err)
	}
	moved, err := s.MoveBoard(f, a.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = moved
	list, err = s.ListBoards()
	if err != nil || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("after down: %+v", list)
	}
	same, err := s.MoveBoard(f, a.ID, 1)
	if err != nil || same.ID != a.ID {
		t.Fatalf("end noop: %+v err=%v", same, err)
	}
	list, err = s.ListBoards()
	if err != nil || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("still at end: %+v", list)
	}
	up, err := s.MoveBoard(f, a.ID, -1)
	if err != nil || up.ID != a.ID {
		t.Fatal(err)
	}
	list, err = s.ListBoards()
	if err != nil || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("after up: %+v", list)
	}
}

func TestUpdateDisplayNameAndOwnPassword(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("oldpass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "mecodeaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("mecodeaaaaaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, _, err := s.CreateThread(b.ID, mem.ID, "帖", "正文")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDisplayName(mem.ID, "   "); err != forum.ErrDisplayNameEmpty {
		t.Fatalf("empty display: %v", err)
	}
	got, err := s.UpdateDisplayName(mem.ID, " 大王 ")
	if err != nil || got.DisplayName != "大王" || got.LoginName != "wang" {
		t.Fatalf("display: %+v err=%v", got, err)
	}
	posts, err := s.ListPosts(th.ID)
	if err != nil || len(posts) != 1 || posts[0].AuthorDisplayName != "大王" || posts[0].AuthorLoginName != "wang" {
		t.Fatalf("old post names: %+v err=%v", posts, err)
	}

	if err := s.CreateSession(mem.ID, "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	nh, err := forum.HashPassword("newpass")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeOwnPassword(mem.ID, nh); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MemberBySession("tok"); err != forum.ErrNotFound {
		t.Fatalf("session after own password: %v", err)
	}
	_, hash, err := s.MemberByLogin("wang")
	if err != nil {
		t.Fatal(err)
	}
	if !forum.CheckPassword(hash, "newpass") || forum.CheckPassword(hash, "oldpass") {
		t.Fatal("password not replaced")
	}
}

func TestSearchThreads(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "searchcodeaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("searchcodeaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	open, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	closed, err := s.CreateBoard("密室", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBoardDisabled(f, closed.ID, true); err != nil {
		t.Fatal(err)
	}

	titleHit, _, err := s.CreateThread(open.ID, mem.ID, "苹果派", "普通正文")
	if err != nil {
		t.Fatal(err)
	}
	bodyHit, _, err := s.CreateThread(open.ID, mem.ID, "别的标题", "这里有香蕉")
	if err != nil {
		t.Fatal(err)
	}
	hiddenBody, p1, err := s.CreateThread(open.ID, mem.ID, "可见标题", "一楼普通")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.CreatePost(hiddenBody.ID, mem.ID, "密语芒果")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPostHidden(f, secret.ID, true); err != nil {
		t.Fatal(err)
	}
	hiddenThread, hp, err := s.CreateThread(open.ID, mem.ID, "藏起来的苹果", "一楼也有苹果")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPostHidden(f, hp.ID, true); err != nil {
		t.Fatal(err)
	}
	inClosed, _, err := s.CreateThread(closed.ID, f.ID, "密室苹果", "只有运营者")
	if err != nil {
		t.Fatal(err)
	}
	_ = p1

	if _, err := s.SearchThreads(mem, "   "); err != forum.ErrSearchEmpty {
		t.Fatalf("empty: %v", err)
	}

	got, err := s.SearchThreads(mem, "苹果")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, v := range got {
		ids[v.ID] = true
	}
	if !ids[titleHit.ID] {
		t.Fatal("member missed title hit")
	}
	if ids[hiddenThread.ID] {
		t.Fatal("member saw hidden thread")
	}
	if ids[inClosed.ID] {
		t.Fatal("member saw disabled board")
	}

	got, err = s.SearchThreads(mem, "香蕉")
	if err != nil || len(got) != 1 || got[0].ID != bodyHit.ID {
		t.Fatalf("body hit: %+v err=%v", got, err)
	}
	got, err = s.SearchThreads(mem, "芒果")
	if err != nil || len(got) != 0 {
		t.Fatalf("member saw hidden floor: %+v err=%v", got, err)
	}

	got, err = s.SearchThreads(f, "芒果")
	if err != nil || len(got) != 1 || got[0].ID != hiddenBody.ID {
		t.Fatalf("staff hidden floor: %+v err=%v", got, err)
	}
	got, err = s.SearchThreads(f, "密室苹果")
	if err != nil || len(got) != 1 || got[0].ID != inClosed.ID || !got[0].BoardDisabled {
		t.Fatalf("staff disabled board: %+v err=%v", got, err)
	}
	got, err = s.SearchThreads(f, "藏起来的苹果")
	if err != nil || len(got) != 1 || got[0].ID != hiddenThread.ID {
		t.Fatalf("staff hidden thread: %+v err=%v", got, err)
	}
}

func TestThreadUnreadMarks(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "unreadcodeaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("unreadcodeaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, p1, err := s.CreateThread(b.ID, mem.ID, "主题", "一楼")
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.ListThreads(b.ID, mem)
	if err != nil || len(list) != 1 || !list[0].Unread {
		t.Fatalf("never opened: %+v err=%v", list, err)
	}
	if err := s.MarkThreadRead(mem.ID, th.ID, p1.Floor); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListThreads(b.ID, mem)
	if err != nil || list[0].Unread {
		t.Fatalf("after read: %+v err=%v", list, err)
	}

	p2, err := s.CreatePost(th.ID, f.ID, "二楼")
	if err != nil {
		t.Fatal(err)
	}
	list, err = s.ListThreads(b.ID, mem)
	if err != nil || !list[0].Unread {
		t.Fatalf("after reply: %+v err=%v", list, err)
	}
	if _, err := s.UpdatePost(mem, p1.ID, "改过的一楼"); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListThreads(b.ID, mem)
	if err != nil || !list[0].Unread {
		t.Fatalf("edit should keep unread: %+v err=%v", list, err)
	}

	if err := s.MarkThreadRead(mem.ID, th.ID, p2.Floor); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListThreads(b.ID, mem)
	if err != nil || list[0].Unread {
		t.Fatalf("caught up: %+v err=%v", list, err)
	}

	got, err := s.SearchThreads(mem, "主题")
	if err != nil || len(got) != 1 || got[0].Unread {
		t.Fatalf("search after catch-up: %+v err=%v", got, err)
	}
}

func TestLockThread(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "lockcodeaaaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("lockcodeaaaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, _, err := s.CreateThread(b.ID, mem.ID, "主题", "一楼")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetThreadLocked(mem, th.ID, true); err != forum.ErrCannotLock {
		t.Fatalf("member lock: %v", err)
	}
	got, err := s.SetThreadLocked(f, th.ID, true)
	if err != nil || !got.Locked {
		t.Fatalf("lock: %+v err=%v", got, err)
	}
	if _, err := s.CreatePost(th.ID, mem.ID, "会员回"); err != forum.ErrCannotReply {
		t.Fatalf("member reply locked: %v", err)
	}
	if _, err := s.CreatePost(th.ID, f.ID, "运营回"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdatePost(mem, 1, "改一楼"); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListThreads(b.ID, mem)
	if err != nil || len(list) != 1 || !list[0].Locked {
		t.Fatalf("list locked: %+v err=%v", list, err)
	}
	back, err := s.SetThreadLocked(f, th.ID, false)
	if err != nil || back.Locked {
		t.Fatalf("unlock: %+v err=%v", back, err)
	}
	if _, err := s.CreatePost(th.ID, mem.ID, "解锁后"); err != nil {
		t.Fatal(err)
	}
}

func TestNotifications(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "noticodeaaaaa"); err != nil {
		t.Fatal(err)
	}
	wang, err := s.Register("noticodeaaaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "noticodebbbbb"); err != nil {
		t.Fatal(err)
	}
	zhao, err := s.Register("noticodebbbbb", "zhao", "老赵", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, _, err := s.CreateThread(b.ID, wang.ID, "主题", "一楼 @zhao 你好")
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListNotifications(zhao.ID)
	if err != nil || len(list) != 1 || list[0].Kind != forum.NotifyMention {
		t.Fatalf("mention: %+v err=%v", list, err)
	}
	self, err := s.ListNotifications(wang.ID)
	if err != nil || len(self) != 0 {
		t.Fatalf("self mention: %+v", self)
	}

	if _, err := s.CreatePost(th.ID, zhao.ID, "回你"); err != nil {
		t.Fatal(err)
	}
	self, err = s.ListNotifications(wang.ID)
	if err != nil || len(self) != 1 || self[0].Kind != forum.NotifyReply {
		t.Fatalf("reply: %+v err=%v", self, err)
	}

	n, err := s.UnreadNotificationCount(zhao.ID)
	if err != nil || n != 1 {
		t.Fatalf("unread %d err=%v", n, err)
	}
	got, err := s.MarkNotificationRead(zhao.ID, list[0].ID)
	if err != nil || !got.Read {
		t.Fatalf("mark: %+v err=%v", got, err)
	}
	n, err = s.UnreadNotificationCount(zhao.ID)
	if err != nil || n != 0 {
		t.Fatalf("after read %d", n)
	}

	closed, err := s.CreateBoard("密室", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBoardDisabled(f, closed.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateThread(closed.ID, f.ID, "密", "只有 @wang"); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListNotifications(wang.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range list {
		if n.Kind == forum.NotifyMention && n.ThreadTitle == "密" {
			t.Fatal("member notified in disabled board")
		}
	}
}

func TestListThreadsByAuthor(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "profcodeaaaaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("profcodeaaaaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	open, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	closed, err := s.CreateBoard("密室", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBoardDisabled(f, closed.ID, true); err != nil {
		t.Fatal(err)
	}
	visible, _, err := s.CreateThread(open.ID, mem.ID, "可见", "一楼")
	if err != nil {
		t.Fatal(err)
	}
	hidden, hp, err := s.CreateThread(open.ID, mem.ID, "隐藏主题", "一楼藏")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPostHidden(f, hp.ID, true); err != nil {
		t.Fatal(err)
	}
	inClosed, _, err := s.CreateThread(closed.ID, mem.ID, "密室帖", "密")
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ListThreadsByAuthor(mem, mem.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, v := range got {
		ids[v.ID] = true
	}
	if !ids[visible.ID] {
		t.Fatal("missing visible")
	}
	if ids[hidden.ID] || ids[inClosed.ID] {
		t.Fatalf("member saw hidden or disabled: %+v", got)
	}

	staff, err := s.ListThreadsByAuthor(f, mem.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids = map[int64]bool{}
	for _, v := range staff {
		ids[v.ID] = true
	}
	if !ids[visible.ID] || !ids[hidden.ID] || !ids[inClosed.ID] {
		t.Fatalf("staff missing: %+v", staff)
	}
}

func TestListPostsByAuthor(t *testing.T) {
	s := testStore(t)
	f := founder(t, s)
	h, err := forum.HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueInvite(f, "ownpostcodeaa"); err != nil {
		t.Fatal(err)
	}
	mem, err := s.Register("ownpostcodeaa", "wang", "老王", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBoard("灌水", "")
	if err != nil {
		t.Fatal(err)
	}
	th, p1, err := s.CreateThread(b.ID, f.ID, "别人的主题", "一楼")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreatePost(th.ID, mem.ID, "我的回帖")
	if err != nil {
		t.Fatal(err)
	}
	_ = p1
	ownTh, _, err := s.CreateThread(b.ID, mem.ID, "我的主题", "我的一楼")
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ListPostsByAuthor(mem, mem.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, v := range got {
		ids[v.ID] = true
	}
	if !ids[p2.ID] {
		t.Fatal("missing reply")
	}
	foundOwn := false
	for _, v := range got {
		if v.ThreadID == ownTh.ID && v.Floor == 1 {
			foundOwn = true
		}
	}
	if !foundOwn {
		t.Fatal("missing own first floor")
	}

	other, err := s.ListPostsByAuthor(f, mem.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) < 1 {
		t.Fatal("staff/other should still query")
	}
}
