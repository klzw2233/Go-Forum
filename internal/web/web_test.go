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
	"time"

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
	day := time.Now().UTC().Format("2006-01-02")
	if !strings.Contains(body, day) {
		t.Fatalf("floor missing date %s: %s", day, body)
	}
	res, err = client.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, day) {
		t.Fatalf("board list missing date %s: %s", day, body)
	}
	if !strings.Contains(body, `href="/threads/1?quote=1#reply"`) {
		// this is board list; quote link is on thread page — ignore
	}
	res, err = client.Get(ts.URL + "/threads/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, `href="/threads/1?quote=1#reply"`) {
		t.Fatalf("quote link missing: %s", body)
	}
	res, err = client.Get(ts.URL + "/threads/1?quote=1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "[#1](/threads/1#floor-1) Jimmy jimmy：") {
		t.Fatalf("quote draft missing: %s", body)
	}
	if !strings.Contains(body, "一楼 **粗体**") {
		t.Fatalf("quoted body missing: %s", body)
	}
	res, err = client.Get(ts.URL + "/threads/1?quote=99")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if strings.Contains(body, "> [#99]") {
		t.Fatalf("bad quote leaked: %s", body)
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
	if strings.Contains(body, `quote=2`) {
		t.Fatalf("hidden floor has quote: %s", body)
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

func TestEditTitleHTTP(t *testing.T) {
	ts, client := testServer(t)
	loginFounder(t, ts, client)
	res, err := client.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
	res, err = client.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"旧标题"}, "body": {"一楼"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	if !strings.Contains(body, "/threads/1/title") {
		t.Fatalf("edit title link missing: %s", body)
	}
	res, err = client.PostForm(ts.URL+"/threads/1/title", url.Values{"title": {"新标题"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	if !strings.Contains(body, "新标题") || strings.Contains(body, ">旧标题<") {
		t.Fatalf("title not updated: %s", body)
	}
	if !strings.Contains(body, "标题已改") {
		t.Fatalf("mark missing: %s", body)
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
		t.Fatalf("code: %s", body)
	}
	code := m[1]
	res, err = client.PostForm(ts.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = client.PostForm(ts.URL+"/register", url.Values{
		"code": {code}, "login_name": {"wang"}, "display_name": {"老王"}, "password": {"hunter2"},
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
	if !strings.Contains(body, "新标题") || !strings.Contains(body, "标题已改") {
		t.Fatalf("member view: %s", body)
	}
	res, err = client.Get(ts.URL + "/threads/1/title")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member edit title %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestPinHTTP(t *testing.T) {
	ts, client := testServer(t)
	loginFounder(t, ts, client)
	res, err := client.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
	res, err = client.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"A"}, "body": {"a"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
	res, err = client.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"B"}, "body": {"b"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	res, err = client.PostForm(ts.URL+"/threads/2/pin", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	if !strings.Contains(body, "置顶") {
		t.Fatalf("pin badge missing: %s", body)
	}
	bPos := strings.Index(body, ">B<")
	aPos := strings.Index(body, ">A<")
	if bPos < 0 || aPos < 0 || bPos > aPos {
		t.Fatalf("B should be above A: %s", body)
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
		t.Fatalf("code: %s", body)
	}
	code := m[1]
	res, err = client.PostForm(ts.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = client.PostForm(ts.URL+"/register", url.Values{
		"code": {code}, "login_name": {"wang"}, "display_name": {"老王"}, "password": {"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
	res, err = client.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if strings.Contains(body, "action=\"/threads/2/pin\"") || strings.Contains(body, ">置顶</button>") && strings.Contains(body, "pin-up") {
		// member should not see pin controls; badge 置顶 is ok
	}
	if strings.Contains(body, "pin-up") {
		t.Fatalf("member saw pin controls: %s", body)
	}
	res, err = client.PostForm(ts.URL+"/threads/1/pin", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member pin %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestMoveThreadHTTP(t *testing.T) {
	ts, client := testServer(t)
	loginFounder(t, ts, client)
	res, err := client.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
	res, err = client.PostForm(ts.URL+"/boards/new", url.Values{"name": {"技术"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
	res, err = client.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"挪我"}, "body": {"一楼"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	if !strings.Contains(body, "/threads/1/move") {
		t.Fatalf("move link missing: %s", body)
	}

	res, err = client.Get(ts.URL + "/threads/1/move")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "技术") {
		t.Fatalf("move page: %d %s", res.StatusCode, body)
	}

	res, err = client.PostForm(ts.URL+"/threads/1/move", url.Values{"board_id": {"2"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body = readBody(t, res)
	if !strings.Contains(body, "技术") {
		t.Fatalf("should land on new board: %s", body)
	}

	res, err = client.Get(ts.URL + "/boards/2")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "挪我") {
		t.Fatalf("thread missing in new board: %s", body)
	}
	res, err = client.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if strings.Contains(body, "挪我") {
		t.Fatalf("thread still in old board: %s", body)
	}

	registerMember(t, ts, client, "wang", "老王")
	res, err = client.Get(ts.URL + "/threads/1/move")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member move page %d", res.StatusCode)
	}
	res.Body.Close()
	res, err = client.PostForm(ts.URL+"/threads/1/move", url.Values{"board_id": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member move post %d", res.StatusCode)
	}
	res.Body.Close()
}

func registerMember(t *testing.T, ts *httptest.Server, client *http.Client, login, display string) {
	t.Helper()
	res, err := client.PostForm(ts.URL+"/invites", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	m := regexp.MustCompile(`刚发出：<code class="invite">([^<]+)</code>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("invite code missing: %s", body)
	}
	if res, err = client.PostForm(ts.URL+"/logout", nil); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = client.PostForm(ts.URL+"/register", url.Values{
		"code":         {m[1]},
		"login_name":   {login},
		"display_name": {display},
		"password":     {"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
}

func TestDisableBoardHidesFromMembers(t *testing.T) {
	ts, client := testServer(t)
	loginFounder(t, ts, client)

	// Board 1 with a thread.
	res, err := client.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
	res, err = client.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"帖子"}, "body": {"正文"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)

	// Disable it as founder.
	res, err = client.PostForm(ts.URL+"/boards/1/disable", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	body := readBody(t, res)
	if !strings.Contains(body, "已停用") || !strings.Contains(body, "/boards/1/enable") {
		t.Fatalf("founder board page after disable: %s", body)
	}

	// Register a plain member (this logs them in).
	registerMember(t, ts, client, "wang", "老王")

	// Member: board gone from home, board/thread/reply/new all 404.
	res, err = client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readBody(t, res), "灌水") {
		t.Fatal("member saw disabled board on home")
	}
	for _, path := range []string{"/boards/1", "/boards/1/threads/new", "/threads/1"} {
		res, err = client.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("member GET %s = %d, want 404", path, res.StatusCode)
		}
		res.Body.Close()
	}
	res, err = client.PostForm(ts.URL+"/threads/1/posts", url.Values{"body": {"回复"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("member reply in disabled board = %d, want 404", res.StatusCode)
	}
	res.Body.Close()

	// Member cannot disable/enable.
	res, err = client.PostForm(ts.URL+"/boards/1/enable", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member enable = %d, want 403", res.StatusCode)
	}
	res.Body.Close()

	// Founder re-enables; member sees it again.
	loginFounder(t, ts, client)
	res, err = client.PostForm(ts.URL+"/boards/1/enable", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	_ = readBody(t, res)
	res, err = client.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, client, res)
	if !strings.Contains(readBody(t, res), "灌水") {
		t.Fatal("member should see re-enabled board")
	}
	res, err = client.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("member GET re-enabled board = %d", res.StatusCode)
	}
	res.Body.Close()
}

func newClient() *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestSuspendMember(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)

	res, err := founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	res, err = founderC.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"旧帖"}, "body": {"还在"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)

	res, err = founderC.PostForm(ts.URL+"/invites", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	body := readBody(t, res)
	m := regexp.MustCompile(`刚发出：<code class="invite">([^<]+)</code>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("invite missing: %s", body)
	}

	wangC := newClient()
	res, err = wangC.PostForm(ts.URL+"/register", url.Values{
		"code": {m[1]}, "login_name": {"wang"}, "display_name": {"老王"}, "password": {"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, wangC, res)
	_ = readBody(t, res)

	res, err = wangC.Get(ts.URL + "/members")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member GET /members = %d", res.StatusCode)
	}
	res.Body.Close()
	res, err = wangC.PostForm(ts.URL+"/members/1/suspend", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member POST suspend = %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.Get(ts.URL + "/members")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "wang") || !strings.Contains(body, "/members/2/suspend") {
		t.Fatalf("members page: %s", body)
	}
	res, err = founderC.PostForm(ts.URL+"/members/1/suspend", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("suspend founder = %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.PostForm(ts.URL+"/members/2/suspend", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	body = readBody(t, res)
	if !strings.Contains(body, "已停用") || !strings.Contains(body, "/members/2/unsuspend") {
		t.Fatalf("after suspend: %s", body)
	}

	res, err = wangC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("suspended session: %d loc=%s", res.StatusCode, res.Header.Get("Location"))
	}
	res.Body.Close()

	res, err = wangC.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "此账号已停用") {
		t.Fatalf("suspended login: %d %s", res.StatusCode, body)
	}

	res, err = founderC.Get(ts.URL + "/threads/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "还在") {
		t.Fatalf("old post gone: %s", body)
	}

	res, err = founderC.PostForm(ts.URL+"/members/2/unsuspend", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	res, err = wangC.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, wangC, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login after restore: %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestPromoteAndSetPassword(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)

	res, err := founderC.PostForm(ts.URL+"/invites", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	body := readBody(t, res)
	m := regexp.MustCompile(`刚发出：<code class="invite">([^<]+)</code>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("invite missing: %s", body)
	}

	wangC := newClient()
	res, err = wangC.PostForm(ts.URL+"/register", url.Values{
		"code": {m[1]}, "login_name": {"wang"}, "display_name": {"老王"}, "password": {"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, wangC, res)
	_ = readBody(t, res)

	res, err = wangC.PostForm(ts.URL+"/members/2/promote", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member promote = %d", res.StatusCode)
	}
	res.Body.Close()
	res, err = wangC.Get(ts.URL + "/members/1/password")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member set founder password GET = %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.PostForm(ts.URL+"/members/1/promote", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("promote founder = %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.PostForm(ts.URL+"/members/2/promote", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	body = readBody(t, res)
	if !strings.Contains(body, "运营者") || !strings.Contains(body, "/members/2/demote") {
		t.Fatalf("after promote: %s", body)
	}

	res, err = wangC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home := readBody(t, res)
	if !strings.Contains(home, "运营者") {
		t.Fatalf("promoted session badge: %s", home)
	}

	res, err = wangC.Get(ts.URL + "/members/1/password")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("operator set founder password = %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.Get(ts.URL + "/members/2/password")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "wang") {
		t.Fatalf("password form: %d %s", res.StatusCode, body)
	}
	res, err = founderC.PostForm(ts.URL+"/members/2/password", url.Values{"password": {"newpass"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)

	res, err = wangC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("session after set password: %d loc=%s", res.StatusCode, res.Header.Get("Location"))
	}
	res.Body.Close()
	res, err = wangC.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "登录名或密码不对") {
		t.Fatalf("old password still works: %s", body)
	}
	res, err = wangC.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"newpass"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, wangC, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("new password login: %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.PostForm(ts.URL+"/members/2/demote", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	body = readBody(t, res)
	if !strings.Contains(body, "/members/2/promote") {
		t.Fatalf("after demote: %s", body)
	}
}

func TestEditAndReorderBoards(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)

	res, err := founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {"闲聊"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	res, err = founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"技术"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)

	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home := readBody(t, res)
	if !strings.Contains(home, "/boards/1/up") || !strings.Contains(home, "/boards/2/down") {
		t.Fatalf("reorder buttons missing: %s", home)
	}
	i1 := strings.Index(home, "灌水")
	i2 := strings.Index(home, "技术")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Fatalf("initial order: %s", home)
	}

	res, err = founderC.Get(ts.URL + "/boards/1/edit")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, `value="灌水"`) {
		t.Fatalf("edit form: %d %s", res.StatusCode, body)
	}
	res, err = founderC.PostForm(ts.URL+"/boards/1/edit", url.Values{"name": {"水区"}, "description": {"随便聊"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	body = readBody(t, res)
	if !strings.Contains(body, "水区") || !strings.Contains(body, "随便聊") {
		t.Fatalf("after rename: %s", body)
	}

	res, err = founderC.PostForm(ts.URL+"/boards/1/down", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	home = readBody(t, res)
	i1 = strings.Index(home, "水区")
	i2 = strings.Index(home, "技术")
	if i1 < 0 || i2 < 0 || i2 > i1 {
		t.Fatalf("after down: %s", home)
	}

	registerMember(t, ts, founderC, "wang", "老王")
	res, err = founderC.Get(ts.URL + "/boards/1/edit")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member edit GET = %d", res.StatusCode)
	}
	res.Body.Close()
	res, err = founderC.PostForm(ts.URL+"/boards/1/up", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member up = %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home = readBody(t, res)
	if strings.Contains(home, "/boards/1/up") {
		t.Fatalf("member saw reorder buttons: %s", home)
	}
	i1 = strings.Index(home, "水区")
	i2 = strings.Index(home, "技术")
	if i1 < 0 || i2 < 0 || i2 > i1 {
		t.Fatalf("member order: %s", home)
	}
}

func TestMeDisplayAndPassword(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)

	res, err := founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	registerMember(t, ts, founderC, "wang", "老王")
	res, err = founderC.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"帖"}, "body": {"正文"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)

	res, err = founderC.Get(ts.URL + "/me")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, `value="老王"`) || !strings.Contains(body, "wang") {
		t.Fatalf("me page: %d %s", res.StatusCode, body)
	}
	res, err = founderC.PostForm(ts.URL+"/me/display", url.Values{"display_name": {""}})
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "显示名不能为空") {
		t.Fatalf("empty display: %s", body)
	}
	res, err = founderC.PostForm(ts.URL+"/me/display", url.Values{"display_name": {"大王"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	body = readBody(t, res)
	if !strings.Contains(body, `value="大王"`) {
		t.Fatalf("after display: %s", body)
	}
	res, err = founderC.Get(ts.URL + "/threads/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "大王") || !strings.Contains(body, `<b class="login">wang</b>`) {
		t.Fatalf("old post names: %s", body)
	}
	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home := readBody(t, res)
	if !strings.Contains(home, `href="/me"`) || !strings.Contains(home, "大王") {
		t.Fatalf("nav: %s", home)
	}

	other := newClient()
	res, err = other.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, other, res)
	_ = readBody(t, res)

	res, err = founderC.PostForm(ts.URL+"/me/password", url.Values{"old_password": {"wrong"}, "password": {"newpass"}})
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "旧密码不对") {
		t.Fatalf("bad old password: %s", body)
	}
	res, err = founderC.PostForm(ts.URL+"/me/password", url.Values{"old_password": {"hunter2"}, "password": {"newpass"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("after password change: %d", res.StatusCode)
	}
	_ = readBody(t, res)

	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("current session after password: %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = other.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("other session: %d loc=%s", res.StatusCode, res.Header.Get("Location"))
	}
	res.Body.Close()
	res, err = other.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "登录名或密码不对") {
		t.Fatalf("old password still works: %s", body)
	}
	res, err = other.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"newpass"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, other, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("new password login: %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestSearchVisibility(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)

	res, err := founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	res, err = founderC.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"苹果派"}, "body": {"一楼"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	res, err = founderC.PostForm(ts.URL+"/threads/1/posts", url.Values{"body": {"密语芒果"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	res, err = founderC.PostForm(ts.URL+"/posts/2/hide", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)

	res, err = founderC.Get(ts.URL + "/search?q=苹果")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "苹果派") {
		t.Fatalf("founder title search: %d %s", res.StatusCode, body)
	}
	if !strings.Contains(body, time.Now().UTC().Format("2006-01-02")) {
		t.Fatalf("search missing date: %s", body)
	}
	res, err = founderC.Get(ts.URL + "/search?q=芒果")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "苹果派") {
		t.Fatalf("founder hidden floor search: %s", body)
	}

	registerMember(t, ts, founderC, "wang", "老王")
	res, err = founderC.Get(ts.URL + "/search?q=苹果")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "苹果派") {
		t.Fatalf("member title search: %s", body)
	}
	res, err = founderC.Get(ts.URL + "/search?q=芒果")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if strings.Contains(body, "苹果派") {
		t.Fatalf("member saw hidden floor: %s", body)
	}
	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home := readBody(t, res)
	if !strings.Contains(home, `action="/search"`) {
		t.Fatalf("search box missing: %s", home)
	}
}

func TestUnreadBadge(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)

	res, err := founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	registerMember(t, ts, founderC, "wang", "老王")
	res, err = founderC.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"主题"}, "body": {"一楼"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create thread %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if !strings.Contains(body, "未读") {
		t.Fatalf("member should see unread: %s", body)
	}

	res, err = founderC.Get(ts.URL + "/threads/1")
	if err != nil {
		t.Fatal(err)
	}
	_ = readBody(t, res)
	res, err = founderC.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if strings.Contains(body, "未读") {
		t.Fatalf("after open still unread: %s", body)
	}

	loginFounder(t, ts, founderC)
	res, err = founderC.PostForm(ts.URL+"/threads/1/posts", url.Values{"body": {"二楼"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)

	res, err = founderC.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	res, err = founderC.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "未读") {
		t.Fatalf("after reply not unread: %s", body)
	}
}

func TestLockThreadHTTP(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)
	res, err := founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	registerMember(t, ts, founderC, "wang", "老王")
	res, err = founderC.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"主题"}, "body": {"一楼"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("new thread %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.PostForm(ts.URL+"/threads/1/lock", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("member lock %d", res.StatusCode)
	}
	res.Body.Close()

	loginFounder(t, ts, founderC)
	res, err = founderC.PostForm(ts.URL+"/threads/1/lock", nil)
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	body := readBody(t, res)
	if !strings.Contains(body, "已锁定") || !strings.Contains(body, "/threads/1/unlock") {
		t.Fatalf("founder lock page: %s", body)
	}

	res, err = founderC.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	res, err = founderC.Get(ts.URL + "/threads/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "已锁定，不能回帖") || strings.Contains(body, `action="/threads/1/posts"`) {
		t.Fatalf("member locked thread: %s", body)
	}
	if strings.Contains(body, "quote=") {
		t.Fatalf("member locked no quote: %s", body)
	}
	res, err = founderC.PostForm(ts.URL+"/threads/1/posts", url.Values{"body": {"硬回"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusForbidden {
		body = readBody(t, res)
		t.Fatalf("member reply locked %d %s", res.StatusCode, body)
	}
	if res.StatusCode == http.StatusOK {
		body = readBody(t, res)
		if !strings.Contains(body, "已锁定") {
			t.Fatalf("member reply body: %s", body)
		}
	} else {
		res.Body.Close()
	}

	res, err = founderC.Get(ts.URL + "/boards/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "已锁定") {
		t.Fatalf("board list: %s", body)
	}
}

func TestMentionNotifyHTTP(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)
	res, err := founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	registerMember(t, ts, founderC, "wang", "老王")
	res, err = founderC.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"主题"}, "body": {"@jimmy 看这里"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("new thread %d", res.StatusCode)
	}
	res.Body.Close()

	loginFounder(t, ts, founderC)
	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home := readBody(t, res)
	if !strings.Contains(home, `href="/notifications"`) || !strings.Contains(home, ">1<") {
		t.Fatalf("unread badge: %s", home)
	}
	res, err = founderC.Get(ts.URL + "/notifications")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if !strings.Contains(body, "点了你的名") || !strings.Contains(body, "主题") {
		t.Fatalf("list: %s", body)
	}
	res, err = founderC.PostForm(ts.URL+"/notifications/1/read", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("read %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "/threads/1") {
		t.Fatalf("location %s", loc)
	}
	res.Body.Close()
}

func TestProfilePage(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)
	res, err := founderC.PostForm(ts.URL+"/boards/new", url.Values{"name": {"灌水"}, "description": {""}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)
	registerMember(t, ts, founderC, "wang", "老王")
	res, err = founderC.PostForm(ts.URL+"/boards/1/threads/new", url.Values{"title": {"可见主题"}, "body": {"一楼 @jimmy"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("thread %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.Get(ts.URL + "/u/wang")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "可见主题") || !strings.Contains(body, "wang") {
		t.Fatalf("profile: %d %s", res.StatusCode, body)
	}
	if !strings.Contains(body, time.Now().UTC().Format("2006-01-02")) {
		t.Fatalf("profile threads missing date: %s", body)
	}
	if !strings.Contains(body, "?tab=posts") {
		t.Fatalf("own profile missing posts tab: %s", body)
	}
	res, err = founderC.Get(ts.URL + "/u/wang?tab=posts")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, "可见主题 #1") {
		t.Fatalf("own posts tab: %s", body)
	}
	if !strings.Contains(body, time.Now().UTC().Format("2006-01-02")) {
		t.Fatalf("profile posts missing date: %s", body)
	}
	res, err = founderC.Get(ts.URL + "/u/nosuch")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.Get(ts.URL + "/threads/1")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if !strings.Contains(body, `href="/u/wang"`) {
		t.Fatalf("floor link: %s", body)
	}
	if !strings.Contains(body, `<a href="/u/jimmy">@jimmy</a>`) {
		t.Fatalf("mention link: %s", body)
	}
}

func TestMessagesHTTP(t *testing.T) {
	ts, founderC := testServer(t)
	loginFounder(t, ts, founderC)
	registerMember(t, ts, founderC, "wang", "老王")

	loginFounder(t, ts, founderC)
	res, err := founderC.Get(ts.URL + "/u/wang")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if !strings.Contains(body, `href="/messages/u/wang"`) {
		t.Fatalf("profile link: %s", body)
	}

	res, err = founderC.Get(ts.URL + "/messages/u/jimmy")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("self %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = founderC.PostForm(ts.URL+"/messages/u/wang", url.Values{"body": {"你好 **王**"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("send %d %s", res.StatusCode, readBody(t, res))
	}
	res.Body.Close()

	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home := readBody(t, res)
	if !strings.Contains(home, `href="/messages"`) {
		t.Fatalf("nav: %s", home)
	}
	if strings.Contains(home, `href="/messages"`) && strings.Contains(home[strings.Index(home, `href="/messages"`):strings.Index(home, `href="/messages"`)+80], ">1<") {
		t.Fatalf("sender should not have unread: %s", home)
	}

	res, err = founderC.PostForm(ts.URL+"/login", url.Values{"login_name": {"wang"}, "password": {"hunter2"}})
	if err != nil {
		t.Fatal(err)
	}
	res = follow(t, founderC, res)
	_ = readBody(t, res)

	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home = readBody(t, res)
	if !strings.Contains(home, `href="/messages"`) || !strings.Contains(home, ">1<") {
		t.Fatalf("wang unread badge: %s", home)
	}

	res, err = founderC.Get(ts.URL + "/messages")
	if err != nil {
		t.Fatal(err)
	}
	inbox := readBody(t, res)
	if !strings.Contains(inbox, "未读") || !strings.Contains(inbox, "jimmy") {
		t.Fatalf("inbox: %s", inbox)
	}

	res, err = founderC.Get(ts.URL + "/messages/u/jimmy")
	if err != nil {
		t.Fatal(err)
	}
	thread := readBody(t, res)
	if !strings.Contains(thread, "<strong>王</strong>") {
		t.Fatalf("markdown: %s", thread)
	}

	res, err = founderC.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	home = readBody(t, res)
	if strings.Contains(home, ">1<") && strings.Contains(home, `href="/messages"`) {
		// badge may still show notices; require messages badge specifically gone
		idx := strings.Index(home, `href="/messages"`)
		chunk := home[idx : idx+90]
		if strings.Contains(chunk, ">1<") {
			t.Fatalf("after open still unread: %s", chunk)
		}
	}

	res, err = founderC.PostForm(ts.URL+"/messages/u/jimmy", url.Values{"body": {""}})
	if err != nil {
		t.Fatal(err)
	}
	empty := readBody(t, res)
	if !strings.Contains(empty, "正文不能为空") {
		t.Fatalf("empty body: %s", empty)
	}
}
