# 升/降运营者，给别人设密码

都在「会员」名单页。没有个人页。

升/降：只有创始人。`POST /members/{id}/promote` 把会员升为运营者，`demote` 把运营者降回会员。已经是目标身份则 noop。创始人身份不能改。运营者点这两个接口 403。

设密码：不知道旧密码。`GET/POST /members/{id}/password`。运营者只能给普通会员设；创始人还能给运营者设。谁都不能给创始人设。设完清掉对方全部 session，对方用新密码重新登录。

已停用的人也能升/降、设密码。自己改密码（要旧密码）不是这一刀。

权限：`forum.CanSetRole`、`forum.CanSetPassword`（后者等于 `CanSuspend`）。
