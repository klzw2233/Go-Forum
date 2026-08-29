# 改标题

作者（一楼未藏时）和创始人/运营者在 `/threads/{id}/title` 改标题。不保存旧标题。会员只看见当前标题和「标题已改」+ UTC 时间。

改标题不更新 `last_post_at`。一楼已隐藏时作者不能改，创始人/运营者可以。

旧库：`ALTER TABLE threads ADD COLUMN title_edited_at TEXT`，列已存在则忽略。
