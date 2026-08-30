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
	ErrInvalidLoginName  = errors.New("invalid login name")
	ErrDisplayNameEmpty  = errors.New("display name is empty")
	ErrDisplayNameLong   = errors.New("display name is too long")
	ErrTitleEmpty        = errors.New("title is empty")
	ErrTitleLong         = errors.New("title is too long")
	ErrBodyEmpty         = errors.New("body is empty")
	ErrBodyLong          = errors.New("body is too long")
	ErrBoardNameEmpty    = errors.New("board name is empty")
	ErrBoardNameLong     = errors.New("board name is too long")
	ErrBoardDescLong     = errors.New("board description is too long")
	ErrNotFound          = errors.New("not found")
	ErrBadPassword       = errors.New("bad password")
	ErrPasswordEmpty     = errors.New("password is empty")
	ErrLoginNameTaken    = errors.New("login name is taken")
	ErrInviteInvalid     = errors.New("invite code is invalid")
	ErrInviteRevoked     = errors.New("invite code is revoked")
	ErrInviteUsed        = errors.New("invite code is already used")
	ErrCannotIssueInvite = errors.New("cannot issue invite codes")
	ErrCannotEditPost    = errors.New("cannot edit this post")
	ErrCannotViewEdits   = errors.New("cannot view edit history")
	ErrCannotHidePost    = errors.New("cannot hide this post")
	ErrCannotReply       = errors.New("cannot reply to this thread")
	ErrCannotEditTitle   = errors.New("cannot edit this title")
	ErrCannotPin         = errors.New("cannot pin threads")
	ErrCannotManageBoard = errors.New("cannot manage boards")
	ErrCannotMoveThread  = errors.New("cannot move threads")
	ErrCannotSuspend     = errors.New("cannot suspend this member")
)

type Member struct {
	ID          int64
	LoginName   string
	DisplayName string
	Role        Role
	Suspended   bool
	CreatedAt   time.Time
}

type Board struct {
	ID          int64
	Name        string
	Description string
	Sort        int
	Disabled    bool
	CreatedAt   time.Time
}

type Thread struct {
	ID            int64
	BoardID       int64
	Title         string
	AuthorID      int64
	CreatedAt     time.Time
	LastPostAt    time.Time
	TitleEditedAt *time.Time
	PinRank       int
}

type Post struct {
	ID           int64
	ThreadID     int64
	AuthorID     int64
	Floor        int
	BodyMarkdown string
	CreatedAt    time.Time
	EditedAt     *time.Time
	Hidden       bool
}

type ThreadView struct {
	Thread
	AuthorLoginName   string
	AuthorDisplayName string
	FirstHidden       bool
}

type PostView struct {
	Post
	AuthorLoginName   string
	AuthorDisplayName string
	AuthorRole        Role
}

type InviteCode struct {
	ID            int64
	Code          string
	IssuedByID    int64
	IssuedByLogin string
	IssuedAt      time.Time
	Revoked       bool
	UsedByLogin   string
	UsedAt        *time.Time
}

func (c InviteCode) Status() string {
	if c.Revoked {
		return "已作废"
	}
	if c.UsedByLogin != "" || c.UsedAt != nil {
		return "已使用"
	}
	return "未使用"
}

type Edit struct {
	ID           int64
	PostID       int64
	BodyMarkdown string
	EditedAt     time.Time
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

// CanSeeBoard reports whether m may see a board and its contents. A disabled
// board is closed to members; operators still see it and can re-enable it.
func CanSeeBoard(m *Member, b *Board) bool {
	if b == nil {
		return false
	}
	return !b.Disabled || CanCreateBoard(m)
}

func CanIssueInvite(m *Member) bool {
	return CanCreateBoard(m)
}

func CanEditPost(m *Member, p *Post) bool {
	return m != nil && p != nil && m.ID == p.AuthorID && !p.Hidden
}

func CanHidePost(m *Member) bool {
	return CanIssueInvite(m)
}

func ThreadHiddenFromMembers(posts []PostView) bool {
	for _, p := range posts {
		if p.Floor == FirstFloor {
			return p.Hidden
		}
	}
	return false
}

func CanViewEdits(m *Member) bool {
	return CanIssueInvite(m)
}

func CanEditTitle(m *Member, th *Thread, firstFloorHidden bool) bool {
	if m == nil || th == nil {
		return false
	}
	if CanHidePost(m) {
		return true
	}
	return m.ID == th.AuthorID && !firstFloorHidden
}

func CanPin(m *Member) bool {
	return CanHidePost(m)
}

func CanMoveThread(m *Member) bool {
	return CanHidePost(m)
}

// CanSuspend reports whether actor may suspend or restore target.
// Nobody may act on themselves or the founder. Operators may only
// suspend ordinary members; the founder may also suspend operators.
func CanSuspend(actor, target *Member) bool {
	if actor == nil || target == nil || actor.ID == target.ID {
		return false
	}
	if target.Role == RoleFounder {
		return false
	}
	if actor.Role == RoleFounder {
		return true
	}
	return actor.Role == RoleOperator && target.Role == RoleMember
}

func (t Thread) Pinned() bool {
	return t.PinRank > 0
}

func InviteUsable(c *InviteCode) error {
	if c == nil {
		return ErrInviteInvalid
	}
	if c.Revoked {
		return ErrInviteRevoked
	}
	if c.UsedByLogin != "" || c.UsedAt != nil {
		return ErrInviteUsed
	}
	return nil
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

func FormatTimeUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04")
}
