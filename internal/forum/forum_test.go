package forum

import (
	"strings"
	"testing"
	"time"
)

func TestMentionedLoginNames(t *testing.T) {
	got := MentionedLoginNames("hi @wang and @jimmy, also @wang again and @1bad and email@x.com @ok_1")
	want := []string{"wang", "jimmy", "ok_1"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if MentionedLoginNames("@") != nil && len(MentionedLoginNames("@")) != 0 {
		t.Fatalf("bare at: %v", MentionedLoginNames("@"))
	}
}

func TestCanMessage(t *testing.T) {
	a := &Member{ID: 1, Role: RoleMember}
	b := &Member{ID: 2, Role: RoleMember}
	if CanMessage(a, a) || CanMessage(nil, b) {
		t.Fatal("self or nil")
	}
	if !CanMessage(a, b) {
		t.Fatal("member to member")
	}
	b.Suspended = true
	if CanMessage(a, b) {
		t.Fatal("suspended target")
	}
	b.Suspended = false
	a.Suspended = true
	if CanMessage(a, b) {
		t.Fatal("suspended sender")
	}
}

func TestValidLoginName(t *testing.T) {
	ok := []string{"a", "jimmy", "Wang", "a1", "a_b", "A0_z", "abcdefghijabcdefghijabcdefghijab"}
	for _, s := range ok {
		if !ValidLoginName(s) {
			t.Errorf("ValidLoginName(%q) = false, want true", s)
		}
	}

	bad := []string{
		"",
		"1abc",
		"_abc",
		"ab-c",
		"ab.c",
		"ab c",
		"王",
		"a王",
		"ab/c",
		strings.Repeat("a", 33),
	}
	for _, s := range bad {
		if ValidLoginName(s) {
			t.Errorf("ValidLoginName(%q) = true, want false", s)
		}
	}
}

func TestNormalizeNewThread(t *testing.T) {
	if _, err := NormalizeTitle(""); err != ErrTitleEmpty {
		t.Errorf("empty title: %v", err)
	}
	if _, err := NormalizeTitle("   "); err != ErrTitleEmpty {
		t.Errorf("blank title: %v", err)
	}
	if _, err := NormalizeBody(""); err != ErrBodyEmpty {
		t.Errorf("empty body: %v", err)
	}
	if _, err := NormalizeBody("\n\t"); err != ErrBodyEmpty {
		t.Errorf("blank body: %v", err)
	}

	title, err := NormalizeTitle("  hello  ")
	if err != nil || title != "hello" {
		t.Errorf("title trim: %q %v", title, err)
	}
	body, err := NormalizeBody("  world  ")
	if err != nil || body != "world" {
		t.Errorf("body trim: %q %v", body, err)
	}
}

func TestNormalizeSearch(t *testing.T) {
	if _, err := NormalizeSearch(""); err != ErrSearchEmpty {
		t.Fatalf("empty: %v", err)
	}
	if _, err := NormalizeSearch("   "); err != ErrSearchEmpty {
		t.Fatalf("blank: %v", err)
	}
	q, err := NormalizeSearch("  hello  ")
	if err != nil || q != "hello" {
		t.Fatalf("trim: %q %v", q, err)
	}
	if _, err := NormalizeSearch(strings.Repeat("a", 81)); err != ErrSearchLong {
		t.Fatalf("long: %v", err)
	}
}

func TestFloorNumbers(t *testing.T) {
	if FirstFloor != 1 {
		t.Fatalf("FirstFloor = %d, want 1", FirstFloor)
	}
	if g := NextFloor(0); g != 1 {
		t.Errorf("NextFloor(0) = %d, want 1", g)
	}
	if g := NextFloor(1); g != 2 {
		t.Errorf("NextFloor(1) = %d, want 2", g)
	}
	if g := NextFloor(5); g != 6 {
		t.Errorf("NextFloor(5) = %d, want 6", g)
	}
}

func TestCanCreateBoard(t *testing.T) {
	if CanCreateBoard(nil) {
		t.Fatal("nil member must not create boards")
	}
	if CanCreateBoard(&Member{Role: RoleMember}) {
		t.Fatal("member must not create boards")
	}
	if !CanCreateBoard(&Member{Role: RoleOperator}) {
		t.Fatal("operator must create boards")
	}
	if !CanCreateBoard(&Member{Role: RoleFounder}) {
		t.Fatal("founder must create boards")
	}
}

func TestCanIssueInvite(t *testing.T) {
	if CanIssueInvite(nil) || CanIssueInvite(&Member{Role: RoleMember}) {
		t.Fatal("member must not issue invites")
	}
	if !CanIssueInvite(&Member{Role: RoleOperator}) || !CanIssueInvite(&Member{Role: RoleFounder}) {
		t.Fatal("operator and founder must issue invites")
	}
}

func TestInviteUsable(t *testing.T) {
	if err := InviteUsable(nil); err != ErrInviteInvalid {
		t.Fatalf("nil: %v", err)
	}
	if err := InviteUsable(&InviteCode{Code: "x"}); err != nil {
		t.Fatalf("fresh: %v", err)
	}
	if err := InviteUsable(&InviteCode{Code: "x", Revoked: true}); err != ErrInviteRevoked {
		t.Fatalf("revoked: %v", err)
	}
	if err := InviteUsable(&InviteCode{Code: "x", UsedByLogin: "wang"}); err != ErrInviteUsed {
		t.Fatalf("used login: %v", err)
	}
	now := time.Now()
	if err := InviteUsable(&InviteCode{Code: "x", UsedAt: &now}); err != ErrInviteUsed {
		t.Fatalf("used at: %v", err)
	}
}

func TestCanEditPost(t *testing.T) {
	p := &Post{AuthorID: 2}
	if CanEditPost(nil, p) || CanEditPost(&Member{ID: 1, Role: RoleFounder}, p) {
		t.Fatal("non-author must not edit")
	}
	if CanEditPost(&Member{ID: 3, Role: RoleOperator}, p) {
		t.Fatal("operator must not edit others")
	}
	if !CanEditPost(&Member{ID: 2, Role: RoleMember}, p) {
		t.Fatal("author must edit")
	}
}

func TestCanViewEdits(t *testing.T) {
	if CanViewEdits(nil) || CanViewEdits(&Member{Role: RoleMember}) {
		t.Fatal("member must not view edits")
	}
	if !CanViewEdits(&Member{Role: RoleOperator}) || !CanViewEdits(&Member{Role: RoleFounder}) {
		t.Fatal("operator and founder must view edits")
	}
}

func TestCanEditHiddenPost(t *testing.T) {
	p := &Post{AuthorID: 2, Hidden: true}
	if CanEditPost(&Member{ID: 2, Role: RoleMember}, p) {
		t.Fatal("author must not edit hidden post")
	}
}

func TestCanReplyLocked(t *testing.T) {
	open := &Thread{}
	locked := &Thread{Locked: true}
	mem := &Member{Role: RoleMember}
	op := &Member{Role: RoleOperator}
	if CanReply(nil, open) || !CanReply(mem, open) {
		t.Fatal("open thread")
	}
	if CanReply(mem, locked) {
		t.Fatal("member must not reply locked")
	}
	if !CanReply(op, locked) || !CanReply(&Member{Role: RoleFounder}, locked) {
		t.Fatal("staff must reply locked")
	}
	if CanLock(nil) || CanLock(mem) || !CanLock(op) {
		t.Fatal("lock permission")
	}
}

func TestCanHidePost(t *testing.T) {
	if CanHidePost(nil) || CanHidePost(&Member{Role: RoleMember}) {
		t.Fatal("member must not hide")
	}
	if !CanHidePost(&Member{Role: RoleOperator}) || !CanHidePost(&Member{Role: RoleFounder}) {
		t.Fatal("operator and founder must hide")
	}
}

func TestThreadUnread(t *testing.T) {
	if ThreadUnread(0, 0, false) {
		t.Fatal("no floors")
	}
	if !ThreadUnread(0, 1, false) {
		t.Fatal("never opened")
	}
	if ThreadUnread(2, 2, true) {
		t.Fatal("caught up")
	}
	if !ThreadUnread(1, 2, true) {
		t.Fatal("new floor")
	}
}

func TestThreadHiddenFromMembers(t *testing.T) {
	if ThreadHiddenFromMembers([]PostView{{Post: Post{Floor: 1, Hidden: false}}}) {
		t.Fatal("visible first floor")
	}
	if !ThreadHiddenFromMembers([]PostView{{Post: Post{Floor: 1, Hidden: true}}, {Post: Post{Floor: 2}}}) {
		t.Fatal("hidden first floor")
	}
}

func TestCanEditTitle(t *testing.T) {
	th := &Thread{AuthorID: 2}
	if CanEditTitle(nil, th, false) {
		t.Fatal("nil")
	}
	if CanEditTitle(&Member{ID: 3, Role: RoleMember}, th, false) {
		t.Fatal("other member")
	}
	if !CanEditTitle(&Member{ID: 2, Role: RoleMember}, th, false) {
		t.Fatal("author")
	}
	if CanEditTitle(&Member{ID: 2, Role: RoleMember}, th, true) {
		t.Fatal("author when first floor hidden")
	}
	if !CanEditTitle(&Member{ID: 1, Role: RoleFounder}, th, true) {
		t.Fatal("founder when hidden")
	}
	if !CanEditTitle(&Member{ID: 9, Role: RoleOperator}, th, false) {
		t.Fatal("operator")
	}
}

func TestCanPin(t *testing.T) {
	if CanPin(nil) || CanPin(&Member{Role: RoleMember}) {
		t.Fatal("member must not pin")
	}
	if !CanPin(&Member{Role: RoleOperator}) || !CanPin(&Member{Role: RoleFounder}) {
		t.Fatal("staff must pin")
	}
}

func TestCanSuspend(t *testing.T) {
	founder := &Member{ID: 1, Role: RoleFounder}
	op := &Member{ID: 2, Role: RoleOperator}
	op2 := &Member{ID: 5, Role: RoleOperator}
	mem := &Member{ID: 3, Role: RoleMember}
	mem2 := &Member{ID: 4, Role: RoleMember}

	if CanSuspend(nil, mem) || CanSuspend(founder, nil) {
		t.Fatal("nil")
	}
	if CanSuspend(founder, founder) || CanSuspend(op, op) || CanSuspend(mem, mem) {
		t.Fatal("self")
	}
	if CanSuspend(op, founder) || CanSuspend(mem, founder) {
		t.Fatal("founder is unsuspendable")
	}
	if CanSuspend(op, op2) || CanSuspend(mem, op) {
		t.Fatal("operator cannot be suspended by non-founder")
	}
	if CanSuspend(mem, mem2) {
		t.Fatal("member cannot suspend")
	}
	if !CanSuspend(founder, op) {
		t.Fatal("founder must suspend operator")
	}
	if !CanSuspend(founder, mem) {
		t.Fatal("founder must suspend member")
	}
	if !CanSuspend(op, mem) {
		t.Fatal("operator must suspend member")
	}
}

func TestCanSetPassword(t *testing.T) {
	founder := &Member{ID: 1, Role: RoleFounder}
	op := &Member{ID: 2, Role: RoleOperator}
	mem := &Member{ID: 3, Role: RoleMember}
	if CanSetPassword(op, op) || CanSetPassword(op, founder) || CanSetPassword(mem, mem) {
		t.Fatal("password matrix deny")
	}
	if !CanSetPassword(founder, op) || !CanSetPassword(founder, mem) || !CanSetPassword(op, mem) {
		t.Fatal("password matrix allow")
	}
}

func TestCanSetRole(t *testing.T) {
	founder := &Member{ID: 1, Role: RoleFounder}
	op := &Member{ID: 2, Role: RoleOperator}
	mem := &Member{ID: 3, Role: RoleMember}
	if CanSetRole(nil, mem) || CanSetRole(founder, nil) || CanSetRole(founder, founder) {
		t.Fatal("nil or self")
	}
	if CanSetRole(op, mem) || CanSetRole(mem, op) {
		t.Fatal("non-founder")
	}
	if !CanSetRole(founder, mem) || !CanSetRole(founder, op) {
		t.Fatal("founder must set role")
	}
}

func TestCanSeeBoard(t *testing.T) {
	open := &Board{}
	closed := &Board{Disabled: true}
	member := &Member{Role: RoleMember}
	op := &Member{Role: RoleOperator}

	if !CanSeeBoard(member, open) || !CanSeeBoard(nil, open) {
		t.Fatal("everyone sees an open board")
	}
	if CanSeeBoard(member, closed) || CanSeeBoard(nil, closed) {
		t.Fatal("members must not see a disabled board")
	}
	if !CanSeeBoard(op, closed) {
		t.Fatal("operators still see a disabled board")
	}
	if CanSeeBoard(member, nil) {
		t.Fatal("nil board is never visible")
	}
}
