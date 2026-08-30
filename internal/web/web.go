package web

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-forum/internal/forum"
	"go-forum/internal/store"
	"go-forum/internal/web/markdown"
)

//go:embed templates/*.html static/*
var embedded embed.FS

const sessionCookie = "forum_session"
const sessionTTL = 7 * 24 * time.Hour

type Server struct {
	store *store.Store
	mux   *http.ServeMux
	tpl   map[string]*template.Template
}

func New(st *store.Store) (*Server, error) {
	s := &Server{store: st, mux: http.NewServeMux(), tpl: map[string]*template.Template{}}
	pages := []string{"login.html", "home.html", "board_new.html", "board.html", "thread_new.html", "thread.html", "thread_move.html", "register.html", "invites.html", "post_edit.html", "post_edits.html", "title_edit.html"}
	for _, p := range pages {
		t, err := template.ParseFS(embedded, "templates/layout.html", "templates/"+p)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		s.tpl[p] = t
	}
	static, err := fs.Sub(embedded, "static")
	if err != nil {
		return nil, err
	}
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /login", s.getLogin)
	s.mux.HandleFunc("POST /login", s.postLogin)
	s.mux.HandleFunc("GET /register", s.getRegister)
	s.mux.HandleFunc("POST /register", s.postRegister)
	s.mux.HandleFunc("POST /logout", s.requireMember(s.postLogout))
	s.mux.HandleFunc("GET /{$}", s.requireMember(s.getHome))
	s.mux.HandleFunc("GET /invites", s.requireMember(s.getInvites))
	s.mux.HandleFunc("POST /invites", s.requireMember(s.postInvites))
	s.mux.HandleFunc("POST /invites/{id}/revoke", s.requireMember(s.postRevokeInvite))
	s.mux.HandleFunc("GET /boards/new", s.requireMember(s.getBoardNew))
	s.mux.HandleFunc("POST /boards/new", s.requireMember(s.postBoardNew))
	s.mux.HandleFunc("GET /boards/{id}", s.requireMember(s.getBoard))
	s.mux.HandleFunc("POST /boards/{id}/disable", s.requireMember(s.postDisableBoard))
	s.mux.HandleFunc("POST /boards/{id}/enable", s.requireMember(s.postEnableBoard))
	s.mux.HandleFunc("GET /boards/{id}/threads/new", s.requireMember(s.getThreadNew))
	s.mux.HandleFunc("POST /boards/{id}/threads/new", s.requireMember(s.postThreadNew))
	s.mux.HandleFunc("GET /threads/{id}", s.requireMember(s.getThread))
	s.mux.HandleFunc("GET /threads/{id}/title", s.requireMember(s.getTitleEdit))
	s.mux.HandleFunc("POST /threads/{id}/title", s.requireMember(s.postTitleEdit))
	s.mux.HandleFunc("POST /threads/{id}/posts", s.requireMember(s.postReply))
	s.mux.HandleFunc("GET /posts/{id}/edit", s.requireMember(s.getPostEdit))
	s.mux.HandleFunc("POST /posts/{id}/edit", s.requireMember(s.postPostEdit))
	s.mux.HandleFunc("GET /posts/{id}/edits", s.requireMember(s.getPostEdits))
	s.mux.HandleFunc("POST /posts/{id}/hide", s.requireMember(s.postHide))
	s.mux.HandleFunc("POST /posts/{id}/unhide", s.requireMember(s.postUnhide))
	s.mux.HandleFunc("POST /threads/{id}/pin", s.requireMember(s.postPin))
	s.mux.HandleFunc("POST /threads/{id}/unpin", s.requireMember(s.postUnpin))
	s.mux.HandleFunc("POST /threads/{id}/pin-up", s.requireMember(s.postPinUp))
	s.mux.HandleFunc("POST /threads/{id}/pin-down", s.requireMember(s.postPinDown))
	s.mux.HandleFunc("GET /threads/{id}/move", s.requireMember(s.getThreadMove))
	s.mux.HandleFunc("POST /threads/{id}/move", s.requireMember(s.postThreadMove))
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

type page struct {
	Member         *forum.Member
	RoleLabel      string
	CanCreateBoard bool
	CanIssueInvite bool
	Error          string
	Code           string
	Boards         []forum.Board
	Board          *forum.Board
	Threads        []forum.ThreadView
	Thread         *forum.Thread
	Posts          []postVM
	Invites        []forum.InviteCode
	NewCode        string
	Post           *forum.Post
	Edits          []editVM
	CanViewEdits   bool
	CanHide        bool
	ThreadHidden   bool
	CanEditTitle   bool
	TitleEdited    string
	CanPin         bool
	CanMoveThread  bool
}

type postVM struct {
	forum.PostView
	BodyHTML    template.HTML
	RoleLabel   string
	CanEdit     bool
	EditedLabel string
	ShowBody    bool
}

type editVM struct {
	forum.Edit
	BodyHTML    template.HTML
	EditedLabel string
}

func (s *Server) currentMember(r *http.Request) *forum.Member {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	m, err := s.store.MemberBySession(c.Value)
	if err != nil {
		return nil
	}
	return m
}

func (s *Server) requireMember(next func(http.ResponseWriter, *http.Request, *forum.Member)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := s.currentMember(r)
		if m == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r, m)
	}
}

func (s *Server) render(w http.ResponseWriter, name string, p page) {
	t := s.tpl[name]
	if t == nil {
		http.Error(w, "missing template", http.StatusInternalServerError)
		return
	}
	if p.Member != nil {
		p.RoleLabel = forum.RoleLabel(p.Member.Role)
		p.CanCreateBoard = forum.CanCreateBoard(p.Member)
		p.CanIssueInvite = forum.CanIssueInvite(p.Member)
		p.CanViewEdits = forum.CanViewEdits(p.Member)
		p.CanHide = forum.CanHidePost(p.Member)
		p.CanPin = forum.CanPin(p.Member)
		p.CanMoveThread = forum.CanMoveThread(p.Member)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout.html", p); err != nil {
		log.Printf("template %s: %v", name, err)
	}
}

func (s *Server) loginAs(w http.ResponseWriter, m *forum.Member) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	if err := s.store.CreateSession(m.ID, token, time.Now().Add(sessionTTL)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return nil
}

func (s *Server) getLogin(w http.ResponseWriter, r *http.Request) {
	if s.currentMember(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "login.html", page{})
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, "login.html", page{Error: "表单无效"})
		return
	}
	login := strings.TrimSpace(r.FormValue("login_name"))
	pass := r.FormValue("password")
	m, hash, err := s.store.MemberByLogin(login)
	if err != nil || !forum.CheckPassword(hash, pass) {
		s.render(w, "login.html", page{Error: "登录名或密码不对"})
		return
	}
	if err := s.loginAs(w, m); err != nil {
		http.Error(w, "session", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) getRegister(w http.ResponseWriter, r *http.Request) {
	if s.currentMember(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "register.html", page{Code: strings.TrimSpace(r.URL.Query().Get("code"))})
}

func (s *Server) postRegister(w http.ResponseWriter, r *http.Request) {
	if s.currentMember(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, "register.html", page{Error: "表单无效"})
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	login := strings.TrimSpace(r.FormValue("login_name"))
	display := r.FormValue("display_name")
	pass := r.FormValue("password")
	hash, err := forum.HashPassword(pass)
	if err != nil {
		s.render(w, "register.html", page{Code: code, Error: publicErr(err)})
		return
	}
	m, err := s.store.Register(code, login, display, hash)
	if err != nil {
		s.render(w, "register.html", page{Code: code, Error: publicErr(err)})
		return
	}
	if err := s.loginAs(w, m); err != nil {
		http.Error(w, "session", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request, _ *forum.Member) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) getHome(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	boards, err := s.store.ListBoards()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !forum.CanCreateBoard(m) {
		visible := boards[:0]
		for _, b := range boards {
			if !b.Disabled {
				visible = append(visible, b)
			}
		}
		boards = visible
	}
	s.render(w, "home.html", page{Member: m, Boards: boards})
}

func (s *Server) getInvites(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	if !forum.CanIssueInvite(m) {
		http.Error(w, "只有创始人或运营者能管理邀请码", http.StatusForbidden)
		return
	}
	list, err := s.store.ListInvites()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "invites.html", page{Member: m, Invites: list, NewCode: r.URL.Query().Get("new")})
}

func (s *Server) postInvites(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	if !forum.CanIssueInvite(m) {
		http.Error(w, "只有创始人或运营者能管理邀请码", http.StatusForbidden)
		return
	}
	var inv *forum.InviteCode
	var err error
	for i := 0; i < 5; i++ {
		code, genErr := newInviteCode()
		if genErr != nil {
			http.Error(w, "invite", http.StatusInternalServerError)
			return
		}
		inv, err = s.store.IssueInvite(m, code)
		if err == nil {
			break
		}
		if err != forum.ErrInviteInvalid {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err != nil {
		http.Error(w, "invite", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/invites?new="+inv.Code, http.StatusSeeOther)
}

func (s *Server) postRevokeInvite(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	if !forum.CanIssueInvite(m) {
		http.Error(w, "只有创始人或运营者能管理邀请码", http.StatusForbidden)
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.store.RevokeInvite(m, id); err != nil {
		http.Error(w, publicErr(err), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/invites", http.StatusSeeOther)
}

func (s *Server) getBoardNew(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	if !forum.CanCreateBoard(m) {
		http.Error(w, "只有创始人或运营者能建版块", http.StatusForbidden)
		return
	}
	s.render(w, "board_new.html", page{Member: m})
}

func (s *Server) postBoardNew(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	if !forum.CanCreateBoard(m) {
		http.Error(w, "只有创始人或运营者能建版块", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, "board_new.html", page{Member: m, Error: "表单无效"})
		return
	}
	b, err := s.store.CreateBoard(r.FormValue("name"), r.FormValue("description"))
	if err != nil {
		s.render(w, "board_new.html", page{Member: m, Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/boards/"+strconv.FormatInt(b.ID, 10), http.StatusSeeOther)
}

func (s *Server) postDisableBoard(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.setBoardDisabled(w, r, m, true)
}

func (s *Server) postEnableBoard(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.setBoardDisabled(w, r, m, false)
}

func (s *Server) setBoardDisabled(w http.ResponseWriter, r *http.Request, m *forum.Member, disabled bool) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.SetBoardDisabled(m, id, disabled)
	switch err {
	case nil:
		http.Redirect(w, r, "/boards/"+strconv.FormatInt(b.ID, 10), http.StatusSeeOther)
	case forum.ErrCannotManageBoard:
		http.Error(w, "只有创始人或运营者能停用版块", http.StatusForbidden)
	case forum.ErrNotFound:
		http.NotFound(w, r)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) getBoard(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.BoardByID(id)
	if err != nil || !forum.CanSeeBoard(m, b) {
		http.NotFound(w, r)
		return
	}
	threads, err := s.store.ListThreads(id, m)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "board.html", page{Member: m, Board: b, Threads: threads})
}

func (s *Server) getThreadNew(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.BoardByID(id)
	if err != nil || !forum.CanSeeBoard(m, b) {
		http.NotFound(w, r)
		return
	}
	s.render(w, "thread_new.html", page{Member: m, Board: b})
}

func (s *Server) postThreadNew(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.BoardByID(id)
	if err != nil || !forum.CanSeeBoard(m, b) {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, "thread_new.html", page{Member: m, Board: b, Error: "表单无效"})
		return
	}
	th, _, err := s.store.CreateThread(b.ID, m.ID, r.FormValue("title"), r.FormValue("body"))
	if err != nil {
		s.render(w, "thread_new.html", page{Member: m, Board: b, Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/threads/"+strconv.FormatInt(th.ID, 10), http.StatusSeeOther)
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	th, err := s.store.ThreadByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.BoardByID(th.BoardID)
	if err != nil || !forum.CanSeeBoard(m, b) {
		http.NotFound(w, r)
		return
	}
	posts, err := s.store.ListPosts(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hidden := forum.ThreadHiddenFromMembers(posts)
	if hidden && !forum.CanHidePost(m) {
		s.render(w, "thread.html", page{Member: m, Board: b, Thread: th, ThreadHidden: true})
		return
	}
	pg := page{Member: m, Board: b, Thread: th, Posts: postVMs(m, posts), ThreadHidden: hidden, CanEditTitle: forum.CanEditTitle(m, th, hidden)}
	if th.TitleEditedAt != nil {
		pg.TitleEdited = "标题已改 " + forum.FormatTimeUTC(*th.TitleEditedAt) + " UTC"
	}
	s.render(w, "thread.html", pg)
}

func (s *Server) postReply(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	th, err := s.store.ThreadByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if b, berr := s.store.BoardByID(th.BoardID); berr != nil || !forum.CanSeeBoard(m, b) {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form", http.StatusBadRequest)
		return
	}
	existing, lerr := s.store.ListPosts(th.ID)
	if lerr != nil {
		http.Error(w, lerr.Error(), http.StatusInternalServerError)
		return
	}
	if forum.ThreadHiddenFromMembers(existing) && !forum.CanHidePost(m) {
		b, _ := s.store.BoardByID(th.BoardID)
		s.render(w, "thread.html", page{Member: m, Board: b, Thread: th, ThreadHidden: true})
		return
	}
	if _, err := s.store.CreatePost(th.ID, m.ID, r.FormValue("body")); err != nil {
		b, _ := s.store.BoardByID(th.BoardID)
		posts, _ := s.store.ListPosts(th.ID)
		s.render(w, "thread.html", page{Member: m, Board: b, Thread: th, Posts: postVMs(m, posts), Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/threads/"+strconv.FormatInt(th.ID, 10), http.StatusSeeOther)
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

func newToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func newInviteCode() (string, error) {
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func publicErr(err error) string {
	switch err {
	case forum.ErrInviteInvalid, forum.ErrNotFound:
		return "邀请码无效"
	case forum.ErrInviteRevoked:
		return "邀请码已作废"
	case forum.ErrInviteUsed:
		return "邀请码已使用"
	case forum.ErrLoginNameTaken:
		return "登录名已被占用"
	case forum.ErrInvalidLoginName:
		return "登录名不合法"
	case forum.ErrDisplayNameEmpty:
		return "显示名不能为空"
	case forum.ErrPasswordEmpty, forum.ErrBadPassword:
		return "密码不能为空"
	case forum.ErrCannotEditPost:
		return "不能编辑这篇帖"
	case forum.ErrCannotViewEdits:
		return "不能查看编辑历史"
	case forum.ErrBodyEmpty:
		return "正文不能为空"
	case forum.ErrCannotHidePost:
		return "不能隐藏这篇帖"
	case forum.ErrCannotReply:
		return "不能回复这篇主题"
	case forum.ErrCannotPin:
		return "不能置顶"
	case forum.ErrCannotManageBoard:
		return "只有创始人或运营者能停用版块"
	case forum.ErrCannotMoveThread:
		return "只有创始人或运营者能挪主题"
	default:
		return err.Error()
	}
}

func postVMs(m *forum.Member, posts []forum.PostView) []postVM {
	vms := make([]postVM, 0, len(posts))
	staff := forum.CanHidePost(m)
	for _, p := range posts {
		show := staff || !p.Hidden
		vm := postVM{
			PostView:  p,
			RoleLabel: forum.RoleLabel(p.AuthorRole),
			CanEdit:   forum.CanEditPost(m, &p.Post),
			ShowBody:  show,
		}
		if show {
			vm.BodyHTML = template.HTML(markdown.Render(p.BodyMarkdown))
			if p.EditedAt != nil {
				vm.EditedLabel = "已编辑 " + forum.FormatTimeUTC(*p.EditedAt) + " UTC"
			}
		}
		vms = append(vms, vm)
	}
	return vms
}

func (s *Server) postHide(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.setHidden(w, r, m, true)
}

func (s *Server) postUnhide(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.setHidden(w, r, m, false)
}

func (s *Server) setHidden(w http.ResponseWriter, r *http.Request, m *forum.Member, hidden bool) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	p, err := s.store.SetPostHidden(m, id, hidden)
	if err != nil {
		if err == forum.ErrCannotHidePost {
			http.Error(w, "不能隐藏这篇帖", http.StatusForbidden)
			return
		}
		if err == forum.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/threads/"+strconv.FormatInt(p.ThreadID, 10)+"#floor-"+strconv.Itoa(p.Floor), http.StatusSeeOther)
}

func (s *Server) getPostEdit(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	p, err := s.store.PostByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !forum.CanEditPost(m, p) {
		http.Error(w, "不能编辑这篇帖", http.StatusForbidden)
		return
	}
	s.render(w, "post_edit.html", page{Member: m, Post: p})
}

func (s *Server) postPostEdit(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	p, err := s.store.PostByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !forum.CanEditPost(m, p) {
		http.Error(w, "不能编辑这篇帖", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, "post_edit.html", page{Member: m, Post: p, Error: "表单无效"})
		return
	}
	updated, err := s.store.UpdatePost(m, p.ID, r.FormValue("body"))
	if err != nil {
		s.render(w, "post_edit.html", page{Member: m, Post: p, Error: publicErr(err)})
		return
	}
	http.Redirect(w, r, "/threads/"+strconv.FormatInt(updated.ThreadID, 10)+"#floor-"+strconv.Itoa(updated.Floor), http.StatusSeeOther)
}

func (s *Server) getPostEdits(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	if !forum.CanViewEdits(m) {
		http.Error(w, "不能查看编辑历史", http.StatusForbidden)
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	p, err := s.store.PostByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	edits, err := s.store.ListEdits(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vms := make([]editVM, 0, len(edits))
	for _, e := range edits {
		vms = append(vms, editVM{
			Edit:        e,
			BodyHTML:    template.HTML(markdown.Render(e.BodyMarkdown)),
			EditedLabel: forum.FormatTimeUTC(e.EditedAt) + " UTC",
		})
	}
	s.render(w, "post_edits.html", page{Member: m, Post: p, Edits: vms})
}

func (s *Server) getTitleEdit(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.titleEdit(w, r, m, false)
}

func (s *Server) postTitleEdit(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.titleEdit(w, r, m, true)
}

func (s *Server) titleEdit(w http.ResponseWriter, r *http.Request, m *forum.Member, save bool) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	th, err := s.store.ThreadByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	posts, err := s.store.ListPosts(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hidden := forum.ThreadHiddenFromMembers(posts)
	if hidden && !forum.CanHidePost(m) {
		http.Error(w, "这篇主题不可见", http.StatusForbidden)
		return
	}
	if !forum.CanEditTitle(m, th, hidden) {
		http.Error(w, "不能修改这个标题", http.StatusForbidden)
		return
	}
	if !save {
		s.render(w, "title_edit.html", page{Member: m, Thread: th})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, "title_edit.html", page{Member: m, Thread: th, Error: "表单无效"})
		return
	}
	updated, err := s.store.UpdateThreadTitle(m, th.ID, r.FormValue("title"))
	if err != nil {
		s.render(w, "title_edit.html", page{Member: m, Thread: th, Error: publicErr(err)})
		return
	}
	http.Redirect(w, r, "/threads/"+strconv.FormatInt(updated.ID, 10), http.StatusSeeOther)
}

func (s *Server) postPin(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.pinAction(w, r, m, "pin")
}

func (s *Server) postUnpin(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.pinAction(w, r, m, "unpin")
}

func (s *Server) postPinUp(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.pinAction(w, r, m, "up")
}

func (s *Server) postPinDown(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	s.pinAction(w, r, m, "down")
}

func (s *Server) pinAction(w http.ResponseWriter, r *http.Request, m *forum.Member, op string) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	var th *forum.Thread
	var err error
	switch op {
	case "pin":
		th, err = s.store.PinThread(m, id)
	case "unpin":
		th, err = s.store.UnpinThread(m, id)
	case "up":
		th, err = s.store.MovePinned(m, id, -1)
	case "down":
		th, err = s.store.MovePinned(m, id, 1)
	default:
		http.NotFound(w, r)
		return
	}
	if err == forum.ErrCannotPin {
		http.Error(w, "不能置顶", http.StatusForbidden)
		return
	}
	if err == forum.ErrNotFound {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/boards/"+strconv.FormatInt(th.BoardID, 10), http.StatusSeeOther)
}

func (s *Server) getThreadMove(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	if !forum.CanMoveThread(m) {
		http.Error(w, "只有创始人或运营者能挪主题", http.StatusForbidden)
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	th, err := s.store.ThreadByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	boards, err := s.store.ListBoards()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "thread_move.html", page{Member: m, Thread: th, Boards: boards})
}

func (s *Server) postThreadMove(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	if !forum.CanMoveThread(m) {
		http.Error(w, "只有创始人或运营者能挪主题", http.StatusForbidden)
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	th, err := s.store.ThreadByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form", http.StatusBadRequest)
		return
	}
	boardID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("board_id")), 10, 64)
	if err != nil || boardID <= 0 {
		boards, berr := s.store.ListBoards()
		if berr != nil {
			http.Error(w, berr.Error(), http.StatusInternalServerError)
			return
		}
		s.render(w, "thread_move.html", page{Member: m, Thread: th, Boards: boards, Error: "要选一个版块"})
		return
	}
	moved, err := s.store.MoveThread(m, th.ID, boardID)
	if err == forum.ErrNotFound {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		boards, berr := s.store.ListBoards()
		if berr != nil {
			http.Error(w, berr.Error(), http.StatusInternalServerError)
			return
		}
		s.render(w, "thread_move.html", page{Member: m, Thread: th, Boards: boards, Error: publicErr(err)})
		return
	}
	http.Redirect(w, r, "/boards/"+strconv.FormatInt(moved.BoardID, 10), http.StatusSeeOther)
}
