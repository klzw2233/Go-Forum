# 私信

两人一条会话。`GET /messages` 列表，`GET/POST /messages/u/{login}` 对话。个人页「发私信」。

不能给自己发。停用会员发不了新的，旧信还在。不删信。正文 Markdown。

未读：`conversation_reads(member_id, conversation_id, last_read_id)`，打开会话标到最新一条。顶栏角标是未读会话数。不写进 notifications 表。
