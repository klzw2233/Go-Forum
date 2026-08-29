package main

import (
	"flag"
	"log"
	"net/http"

	"go-forum/internal/config"
	"go-forum/internal/forum"
	"go-forum/internal/store"
	"go-forum/internal/web"
)

func main() {
	cfgPath := flag.String("config", "forum.toml", "path to forum.toml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if !forum.ValidLoginName(cfg.Founder.LoginName) {
		log.Fatalf("invalid founder login_name %q", cfg.Founder.LoginName)
	}

	st, err := store.Open(cfg.Database)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	hash, err := forum.HashPassword(cfg.Founder.Password)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := st.EnsureFounder(cfg.Founder.LoginName, cfg.Founder.DisplayName, hash); err != nil {
		log.Fatal(err)
	}

	srv, err := web.New(st)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on http://%s", cfg.Listen)
	log.Fatal(http.ListenAndServe(cfg.Listen, srv))
}
