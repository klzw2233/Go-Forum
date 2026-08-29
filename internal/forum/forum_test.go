package forum

import (
	"strings"
	"testing"
	"time"
)

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

func TestCanHidePost(t *testing.T) {
	if CanHidePost(nil) || CanHidePost(&Member{Role: RoleMember}) {
		t.Fatal("member must not hide")
	}
	if !CanHidePost(&Member{Role: RoleOperator}) || !CanHidePost(&Member{Role: RoleFounder}) {
		t.Fatal("operator and founder must hide")
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
