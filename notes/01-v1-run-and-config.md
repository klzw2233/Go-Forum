# 第一刀：启动与配置

## 怎么启动

1. 复制 `forum.example.toml` 为 `forum.toml`（后者已在 `.gitignore`）。
2. 改 `founder.password`。
3. `go run ./cmd/forum`，默认听 `127.0.0.1:8080`。
4. 指定文件：`go run ./cmd/forum -config path/to/forum.toml`。

SQLite 文件默认是当前目录下的 `forum.db`（同样 gitignore）。删掉这个文件等于空库；下次启动会按配置重新插入创始人。

## 配置项

| 键 | 作用 |
|---|---|
| `listen` | HTTP 地址。空则 `127.0.0.1:8080` |
| `database` | SQLite 路径。空则 `forum.db` |
| `founder.login_name` | 创始人登录名，必须通过领域规则 |
| `founder.display_name` | 创始人显示名 |
| `founder.password` | 明文，只用于**首次**创建该登录名。已存在则忽略，避免每次启动把密码改回去 |

没有环境变量覆盖。没有 Docker。一个二进制 + 一个 db 文件。

## 第一刀能力边界

能：登录/登出、创始人建版块、会员发主题和回帖、Markdown（含 https/http 图床出图）。

不能：邀请码、注册、隐藏、编辑历史、置顶、挪版、停用、第二个人登录。
