# 邀请码与注册

未登录只开放 `/login`、`/register`、静态资源。`/register?code=` 会预填邀请码，仍可手改。注册成功后自动登录。

## 码

系统生成 12 字符 URL-safe 串（9 字节随机 → base64url，无 padding）。创始人/运营者在 `/invites` 点「发一张」。未用可作废；已用不能再注册、不能作废。

记下：谁发、何时发、是否作废、被哪个登录名用掉、何时用掉。

## 旧库

`invite_codes` 用 `CREATE TABLE IF NOT EXISTS`。已有 `forum.db` 下次启动会自动加表，不用迁移工具。
