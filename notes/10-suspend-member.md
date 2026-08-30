# 停用会员

创始人/运营者在顶栏「会员」名单里点「停用 / 恢复」。停用不是删除，可随时恢复。

停用后：
- 不能登录，已有 session 立刻失效。
- 登录页对密码正确的停用账号显示「此账号已停用」。
- 以前发的帖仍按原样给会员看，不自动隐藏。
- 登录名仍占用，不能拿去注册。

权限在 `forum.CanSuspend(actor, target)`：不能停自己、不能停创始人；运营者只能停普通会员；创始人还能停运营者。升/降运营者不是这一刀。

旧库：`ALTER TABLE members ADD COLUMN suspended INTEGER NOT NULL DEFAULT 0`。
