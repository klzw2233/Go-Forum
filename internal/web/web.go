package web

import (
	"crypto/rand"
	"embed"
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
	pages := []string{"login.html", "home.html", "board_new.html", "board.html", "thread_new.html", "thread.html"}
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
	s.mux.HandleFunc("POST /logout", s.requireMember(s.postLogout))
	s.mux.HandleFunc("GET /{$}", s.requireMember(s.getHome))
	s.mux.HandleFunc("GET /boards/new", s.requireMember(s.getBoardNew))
	s.mux.HandleFunc("POST /boards/new", s.requireMember(s.postBoardNew))
	s.mux.HandleFunc("GET /boards/{id}", s.requireMember(s.getBoard))
	s.mux.HandleFunc("GET /boards/{id}/threads/new", s.requireMember(s.getThreadNew))
	s.mux.HandleFunc("POST /boards/{id}/threads/new", s.requireMember(s.postThreadNew))
	s.mux.HandleFunc("GET /threads/{id}", s.requireMember(s.getThread))
	s.mux.HandleFunc("POST /threads/{id}/posts", s.requireMember(s.postReply))
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

type page struct {
	Member         *forum.Member
	RoleLabel      string
	CanCreateBoard bool
	Error          string
	Boards         []forum.Board
	Board          *forum.Board
	Threads        []forum.ThreadView
	Thread         *forum.Thread
	Posts          []postVM
}

type postVM struct {
	forum.PostView
	BodyHTML  template.HTML
	RoleLabel string
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
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout.html", p); err != nil {
		log.Printf("template %s: %v", name, err)
	}
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
	token, err := newToken()
	if err != nil {
		http.Error(w, "session", http.StatusInternalServerError)
		return
	}
	if err := s.store.CreateSession(m.ID, token, time.Now().Add(sessionTTL)); err != nil {
		http.Error(w, "session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
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
	s.render(w, "home.html", page{Member: m, Boards: boards})
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

func (s *Server) getBoard(w http.ResponseWriter, r *http.Request, m *forum.Member) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := s.store.BoardByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	threads, err := s.store.ListThreads(id)
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
	if err != nil {
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
	if err != nil {
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
	if err != nil {
		http.NotFound(w, r)
		return
	}
	posts, err := s.store.ListPosts(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vms := make([]postVM, 0, len(posts))
	for _, p := range posts {
		vms = append(vms, postVM{
			PostView:  p,
			BodyHTML:  template.HTML(markdown.Render(p.BodyMarkdown)),
			RoleLabel: forum.RoleLabel(p.AuthorRole),
		})
	}
	s.render(w, "thread.html", page{Member: m, Board: b, Thread: th, Posts: vms})
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form", http.StatusBadRequest)
		return
	}
	if _, err := s.store.CreatePost(th.ID, m.ID, r.FormValue("body")); err != nil {
		b, _ := s.store.BoardByID(th.BoardID)
		posts, _ := s.store.ListPosts(th.ID)
		vms := make([]postVM, 0, len(posts))
		for _, p := range posts {
			vms = append(vms, postVM{
				PostView:  p,
				BodyHTML:  template.HTML(markdown.Render(p.BodyMarkdown)),
				RoleLabel: forum.RoleLabel(p.AuthorRole),
			})
		}
		s.render(w, "thread.html", page{Member: m, Board: b, Thread: th, Posts: vms, Error: err.Error()})
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
