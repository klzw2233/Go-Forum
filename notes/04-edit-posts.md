# 改自己的帖

作者在 `/posts/{id}/edit` 改 Markdown。保存后回到该楼。正文与当前相同则不写历史、不标「已编辑」。

改帖不更新 `threads.last_post_at`，主题不会因此顶到版块列表上面。

会员看见「已编辑」和 UTC 时间。旧全文在 `/posts/{id}/edits`，只有创始人/运营者能打开（直接打 URL 也 403）。历史里每一条是被换掉的那一版，按同一套 Markdown 消毒渲染。

## 旧库

`post_edits` 用 `CREATE TABLE IF NOT EXISTS`。`posts.edited_at` 用 `ALTER TABLE ... ADD COLUMN`，列已存在则忽略。
