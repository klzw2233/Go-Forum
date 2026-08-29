package forum

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type Role string

const (
	RoleMember   Role = "member"
	RoleOperator Role = "operator"
	RoleFounder  Role = "founder"
)

const FirstFloor = 1

const (
	maxLoginName    = 32
	maxDisplayName  = 32
	maxBoardName    = 40
	maxBoardDesc    = 200
	maxThreadTitle  = 120
	maxPostBody     = 64 * 1024
	minLoginNameLen = 1
)

var (
	ErrInvalidLoginName = errors.New("invalid login name")
	ErrDisplayNameEmpty = errors.New("display name is empty")
	ErrDisplayNameLong  = errors.New("display name is too long")
	ErrTitleEmpty       = errors.New("title is empty")
	ErrTitleLong        = errors.New("title is too long")
	ErrBodyEmpty        = errors.New("body is empty")
	ErrBodyLong         = errors.New("body is too long")
	ErrBoardNameEmpty   = errors.New("board name is empty")
	ErrBoardNameLong    = errors.New("board name is too long")
	ErrBoardDescLong    = errors.New("board description is too long")
	ErrNotFound         = errors.New("not found")
	ErrBadPassword      = errors.New("bad password")
)

type Member struct {
	ID          int64
	LoginName   string
	DisplayName string
	Role        Role
	CreatedAt   time.Time
}

type Board struct {
	ID          int64
	Name        string
	Description string
	Sort        int
	CreatedAt   time.Time
}

type Thread struct {
	ID         int64
	BoardID    int64
	Title      string
	AuthorID   int64
	CreatedAt  time.Time
	LastPostAt time.Time
}

type Post struct {
	ID           int64
	ThreadID     int64
	AuthorID     int64
	Floor        int
	BodyMarkdown string
	CreatedAt    time.Time
}

type ThreadView struct {
	Thread
	AuthorLoginName   string
	AuthorDisplayName string
}

type PostView struct {
	Post
	AuthorLoginName   string
	AuthorDisplayName string
	AuthorRole        Role
}

func ValidLoginName(s string) bool {
	if s == "" || len(s) > maxLoginName {
		return false
	}
	if s[0] < 'A' || (s[0] > 'Z' && s[0] < 'a') || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		ok := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_'
		if !ok {
			return false
		}
	}
	return len(s) >= minLoginNameLen
}

func NormalizeDisplayName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrDisplayNameEmpty
	}
	if utf8.RuneCountInString(s) > maxDisplayName {
		return "", ErrDisplayNameLong
	}
	return s, nil
}

func NormalizeBoardName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrBoardNameEmpty
	}
	if utf8.RuneCountInString(s) > maxBoardName {
		return "", ErrBoardNameLong
	}
	return s, nil
}

func NormalizeBoardDesc(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxBoardDesc {
		return "", ErrBoardDescLong
	}
	return s, nil
}

func NormalizeTitle(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrTitleEmpty
	}
	if utf8.RuneCountInString(s) > maxThreadTitle {
		return "", ErrTitleLong
	}
	return s, nil
}

func NormalizeBody(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrBodyEmpty
	}
	if len(s) > maxPostBody {
		return "", ErrBodyLong
	}
	return s, nil
}

func NextFloor(last int) int {
	if last < FirstFloor {
		return FirstFloor
	}
	return last + 1
}

func CanCreateBoard(m *Member) bool {
	return m != nil && (m.Role == RoleFounder || m.Role == RoleOperator)
}

func RoleLabel(r Role) string {
	switch r {
	case RoleFounder:
		return "创始人"
	case RoleOperator:
		return "运营者"
	default:
		return ""
	}
}
