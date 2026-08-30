# 搜索

顶栏输入框 `GET /search?q=`。搜标题和帖正文。空查询不扫库。

会员看不到：停用版、一楼已隐藏的主题、已隐藏楼的正文。运营者能搜到这些，停用版在结果里标「已停用」。停用会员的旧帖会员能搜到。

实现：`forum.NormalizeSearch` + `store.SearchThreads`，SQLite `LIKE`，`%` `_` `\` 转义。无新表。
