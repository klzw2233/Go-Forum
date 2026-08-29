package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRequiresFounder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "forum.toml")
	if err := os.WriteFile(path, []byte("listen = \"127.0.0.1:0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "forum.toml")
	body := `
listen = "127.0.0.1:0"
database = "x.db"
[founder]
login_name = "jimmy"
display_name = "Jimmy"
password = "secret"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:0" || c.Database != "x.db" || c.Founder.LoginName != "jimmy" {
		t.Fatalf("%+v", c)
	}
}
