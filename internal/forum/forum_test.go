package forum

import (
	"strings"
	"testing"
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
