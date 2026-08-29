package web

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go-forum/internal/forum"
	"go-forum/internal/store"
)

func testServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h, err := forum.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureFounder("jimmy", "Jimmy", h); err != nil {
		t.Fatal(err)
	}
	srv, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return ts, client
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func follow(t *testing.T, client *http.Client, res *http.Response) *http.Response {
	t.Helper()
	if res.StatusCode != http.StatusSeeOther && res.StatusCode != http.StatusFound {
		t.Fatalf("status %d body %s", res.StatusCode, readBody(t, res))
	}
	loc := res.Header.Get("Location")
	res.Body.Close()
	ref, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	next, err := client.Get(res.Request.URL.ResolveReference(ref).String())
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func loginFounder(t *testing.T, ts *httptest.Server, client *http.Client) {
	t.Helper()
	res, err := client.PostForm(ts.URL+"/login", url.Values{
		"login_name": {"jimmy"},
		"password":   {"secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
}

func TestUnauthenticatedHomeRedirectsToLogin(t *testing.T) {
	ts, client := testServer(t)
	res, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status %d", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("location %q", loc)
	}
}

func TestRegisterPageOpenAndPrefill(t *testing.T) {
	ts, client := testServer(t)
	res, err := client.Get(ts.URL + "/register?code=abcXYZ123456")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d %s", res.StatusCode, body)
	}
	if !strings.Contains(body, `value="abcXYZ123456"`) {
		t.Fatalf("code not prefilled: %s", body)
	}
}

func TestPostingLoopAndExternalImage(t *testing.T) {
	ts, client := testServer(t)

	res, err := client.PostForm(ts.URL+"/login", url.Values{
		"login_name": {"jimmy"},
		"password":   {"secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("after login status %d %s", res.StatusCode, body)
	}
	if !strings.Contains(body, "版块") {
		t.Fatalf("home missing 版块: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/boards/new", url.Values{
		"name":        {"灌水"},
		"description": {"闲聊"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	if !strings.Contains(body, "灌水") {
		t.Fatalf("board page: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/boards/1/threads/new", url.Values{
		"title": {"第一帖"},
		"body":  {"一楼 **粗体**\n\n![图](https://example.com/a.png)"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	if !strings.Contains(body, "第一帖") {
		t.Fatalf("thread title missing: %s", body)
	}
	if !strings.Contains(body, "<strong>粗体</strong>") && !strings.Contains(body, "<b>粗体</b>") {
		t.Fatalf("markdown missing: %s", body)
	}
	if !strings.Contains(body, `src="https://example.com/a.png"`) {
		t.Fatalf("image missing: %s", body)
	}
	if !strings.Contains(body, `<b class="login">jimmy</b>`) {
		t.Fatalf("login name missing: %s", body)
	}
	if !strings.Contains(body, "创始人") {
		t.Fatalf("founder badge missing: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/threads/1/posts", url.Values{
		"body": {"二楼内容"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	if !strings.Contains(body, "一楼") || !strings.Contains(body, "二楼内容") {
		t.Fatalf("two floors missing: %s", body)
	}
	if !strings.Contains(body, "#1") || !strings.Contains(body, "#2") {
		t.Fatalf("floor numbers missing: %s", body)
	}
}

func TestInviteRegisterAndMemberCannotIssue(t *testing.T) {
	ts, client := testServer(t)
	loginFounder(t, ts, client)

	res, err := client.PostForm(ts.URL+"/invites", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	re := regexp.MustCompile(`刚发出：<code class="invite">([^<]+)</code>`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("new code missing: %s", body)
	}
	code := m[1]
	if len(code) != 12 {
		t.Fatalf("code length %d %q", len(code), code)
	}

	res, err = client.PostForm(ts.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	res, err = client.PostForm(ts.URL+"/register", url.Values{
		"code":         {code},
		"login_name":   {"wang"},
		"display_name": {"老王"},
		"password":     {"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	if !strings.Contains(body, "版块") {
		t.Fatalf("after register: %s", body)
	}
	if !strings.Contains(body, "老王") {
		t.Fatalf("display name missing: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/invites", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member issue status %d", res.StatusCode)
	}
}

func TestRevokedInviteCannotRegister(t *testing.T) {
	ts, client := testServer(t)
	loginFounder(t, ts, client)

	res, err := client.PostForm(ts.URL+"/invites", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	re := regexp.MustCompile(`/invites/(\d+)/revoke`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("revoke form missing: %s", body)
	}
	codeRe := regexp.MustCompile(`刚发出：<code class="invite">([^<]+)</code>`)
	cm := codeRe.FindStringSubmatch(body)
	if cm == nil {
		t.Fatalf("code missing: %s", body)
	}
	code := cm[1]

	res, err = client.PostForm(ts.URL+"/invites/"+m[1]+"/revoke", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	res, err = client.PostForm(ts.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	res, err = client.PostForm(ts.URL+"/register", url.Values{
		"code":         {code},
		"login_name":   {"zhao"},
		"display_name": {"赵"},
		"password":     {"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if !strings.Contains(body, "邀请码已作废") {
		t.Fatalf("expected revoked message: %s", body)
	}
}

func TestEditPostAndHistoryAccess(t *testing.T) {
	ts, client := testServer(t)
	loginFounder(t, ts, client)

	res, err := client.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	res, err = client.PostForm(ts.URL+"/boards/1/threads/new", url.Values{
		"title": {"题"},
		"body":  {"旧文 ![x](https://example.com/old.png)"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	if !strings.Contains(body, "href=\"/posts/1/edit\"") {
		t.Fatalf("edit link missing: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/posts/1/edit", url.Values{"body": {"新文"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	if !strings.Contains(body, "新文") || strings.Contains(body, "旧文") {
		t.Fatalf("body not updated: %s", body)
	}
	if !strings.Contains(body, "已编辑") {
		t.Fatalf("edited mark missing: %s", body)
	}
	if !strings.Contains(body, "href=\"/posts/1/edits\"") {
		t.Fatalf("history link missing: %s", body)
	}

	res, err = client.Get(ts.URL + "/posts/1/edits")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("founder history %d %s", res.StatusCode, body)
	}
	if !strings.Contains(body, "旧文") {
		t.Fatalf("old body missing: %s", body)
	}
	if !strings.Contains(body, `src="https://example.com/old.png"`) {
		t.Fatalf("old image missing: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/invites", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	re := regexp.MustCompile(`刚发出：<code class="invite">([^<]+)</code>`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("code missing: %s", body)
	}
	code := m[1]
	res, err = client.PostForm(ts.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	res, err = client.PostForm(ts.URL+"/register", url.Values{
		"code":         {code},
		"login_name":   {"wang"},
		"display_name": {"老王"},
		"password":     {"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	res, err = client.Get(ts.URL + "/posts/1/edits")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member history %d", res.StatusCode)
	}

	res, err = client.Get(ts.URL + "/posts/1/edit")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member edit others %d", res.StatusCode)
	}
}

func TestHideFloorAndHiddenThreadPage(t *testing.T) {
	ts, client := testServer(t)
	loginFounder(t, ts, client)

	res, err := client.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	res, err = client.PostForm(ts.URL+"/boards/1/threads/new", url.Values{
		"title": {"主题甲"},
		"body":  {"一楼正文"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	res, err = client.PostForm(ts.URL+"/threads/1/posts", url.Values{"body": {"二楼秘密"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	res, err = client.PostForm(ts.URL+"/posts/2/hide", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	if !strings.Contains(body, "取消隐藏") {
		t.Fatalf("staff hide control: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/invites", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	re := regexp.MustCompile(`刚发出：<code class="invite">([^<]+)</code>`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("code missing: %s", body)
	}
	code := m[1]
	res, err = client.PostForm(ts.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = client.PostForm(ts.URL+"/register", url.Values{
		"code":         {code},
		"login_name":   {"wang"},
		"display_name": {"老王"},
		"password":     {"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	res, err = client.Get(ts.URL + "/threads/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "已隐藏") {
		t.Fatalf("placeholder missing: %s", body)
	}
	if strings.Contains(body, "二楼秘密") {
		t.Fatalf("hidden body leaked: %s", body)
	}
	if !strings.Contains(body, "老王") && !strings.Contains(body, "jimmy") {
		t.Fatalf("author missing on placeholder: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/posts/2/hide", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member hide %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = client.PostForm(ts.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	loginFounder(t, ts, client)
	res, err = client.PostForm(ts.URL+"/posts/1/hide", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	if !strings.Contains(body, "一楼正文") {
		t.Fatalf("founder should still see first floor: %s", body)
	}

	res, err = client.PostForm(ts.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = client.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	res, err = client.Get(ts.URL + "/threads/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "这篇主题不可见") {
		t.Fatalf("hidden thread page: %s", body)
	}
	if strings.Contains(body, "一楼正文") {
		t.Fatalf("first floor leaked: %s", body)
	}

	res, err = client.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if strings.Contains(body, "主题甲") {
		t.Fatalf("hidden thread in board list: %s", body)
	}
}
