# Go-Forum

[![CI](https://github.com/klzw2233/Go-Forum/actions/workflows/ci.yml/badge.svg)](https://github.com/klzw2233/Go-Forum/actions/workflows/ci.yml)

熟人封闭传统 BBS。一个站点、一群互相认识的人。未登录什么都看不见。

领域语言见 [CONTEXT.md](CONTEXT.md)。第一版范围见 [docs/adr/0002-v1-scope.md](docs/adr/0002-v1-scope.md)。

## 现在能做什么

配置启动 → 创始人登录 → 建版块 → 发邀请码 → 熟人用码注册（自动登录）→ 开主题 / 回帖 → 改自己的帖。开主题的人和创始人/运营者能改标题（会员看见「标题已改」，看不见旧标题）。创始人/运营者可以把帖对会员隐藏（可撤销），可以在版块列表置顶主题并手排顺序，可以把主题挪到别的版块（楼层、标题、时间都原样带走），也可以停用版块（对会员关门、可启用），也可以改版块名和说明、手排版块顺序，也可以停用会员（不能登录、旧帖原样、可恢复）。创始人可以把会员升为运营者、取消运营者身份；创始人/运营者可以给别人设新密码（运营者只能给普通会员设）。会员可以改显示名，知道旧密码时可以自己改密码。登录后顶栏可以搜标题和帖正文。版块列表和搜索结果会标出未读主题（打开主题页即已读，有新楼再未读）。创始人/运营者可以锁定主题（会员不能回、运营者仍能回）。帖里 `@登录名` 会通知对方并链到个人页；别人回你开的主题也会通知。楼层上的登录名可以打开个人页（他开过的可见主题；自己还能看自己发过的帖）。会员可以给别人发私信。Markdown 里的 `http://` / `https://` 图床地址会当图显示。未登录只能看见登录页和注册页。会员改帖后楼上看见「已编辑」；旧全文只有创始人/运营者能打开。

## 现在不能做什么

搜索对会员不露出隐藏楼、隐藏主题和停用版块。未读按楼号记，改帖不会把主题标回未读。邀请码由系统生成，一次性，可作废，不过期。改帖和改标题都不会把主题顶回版块列表上面。藏一楼后会员打开旧链接会看到「这篇主题不可见」。置顶不改变隐藏可见性。停用版块只对会员关门，不删除内容。停用会员不隐藏旧帖，登录名也不释放。

## 怎么跑

需要 Go 1.22+（本机开发用 1.26）。

```text
copy forum.example.toml forum.toml
```

编辑 `forum.toml`：把 `founder.password` 改成你自己的密码。`forum.toml` 和 `forum.db` 不会进 git。

```text
gofmt -l .
go vet ./...
go test ./...
go run ./cmd/forum
```

推送到 GitHub 后，Actions 会再跑一遍 `gofmt` / `go vet` / `go test`（见 `.github/workflows/ci.yml`）。

浏览器打开 <http://127.0.0.1:8080>，用配置里的登录名和密码登录。

指定配置文件：

```text
go run ./cmd/forum -config forum.toml
```

## 配置项

见 `forum.example.toml`：

| 项 | 含义 |
|---|---|
| `listen` | HTTP 监听地址，默认 `127.0.0.1:8080` |
| `database` | SQLite 文件路径，默认 `forum.db` |
| `founder.login_name` | 创始人登录名（字母开头，只含字母数字下划线） |
| `founder.display_name` | 创始人显示名 |
| `founder.password` | 只在库里还没有这个登录名时用来创建创始人；已存在则不覆盖密码 |

## 仓库结构

```text
cmd/forum/           入口
internal/forum/      领域规则
internal/store/      SQLite
internal/web/        HTTP、模板、Markdown
internal/config/     读 forum.toml
```
