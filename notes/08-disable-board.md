# 停用版块

创始人/运营者在版块页点「停用 / 启用」。停用是对会员关门，不是删除，可随时启用。

停用后：
- 会员的版块列表（首页）不再列出它。
- 会员访问该版、发主题、看/回其中的主题，一律 404（旧链接失效）。
- 运营者仍能进、能发、能看到版块列表里标着「已停用」的它，能启用。

判定集中在 `forum.CanSeeBoard(m, b)`：非停用，或 `m` 是创始人/运营者。所有"进版块内容"的 handler 都过它。

旧库：`ALTER TABLE boards ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`。
