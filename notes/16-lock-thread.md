# 锁主题

创始人/运营者在主题页「锁定 / 解锁」。会员能看旧楼，不能回；运营者仍能回。开主题的人不能锁。

锁不是隐藏：不改可见性、不改楼号、不触发未读。改帖、改标题、置顶、挪版在锁着时仍按原规则。

旧库：`ALTER TABLE threads ADD COLUMN locked INTEGER NOT NULL DEFAULT 0`。
