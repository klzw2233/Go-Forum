# 未读

稀疏表 `thread_reads(member_id, thread_id, last_read_floor)`。没打开过就没行，算未读。

打开主题页（真看见楼，不是「这篇主题不可见」）把 `last_read_floor` 写成当前最大楼号（隐藏楼也占号）。假定从上往下看完，不跟踪跳楼。

未读 = 没行，或 `last_read_floor < MAX(floor)`。新回帖加楼才会未读。改帖、改标题、隐藏不改楼号。

版块列表和搜索结果标「未读」。首页版块不计数。没有全部标已读。
