# TCP 聊天室

一个支持 TCP 和 WebSocket 的实时聊天室。

## 快速开始

```bash
cd tcp-chatroom
go run server.go
```

然后打开浏览器访问 http://localhost:8080

## 连接方式

| 方式 | 地址 | 说明 |
|------|------|------|
| 浏览器 | http://localhost:8080 | WebSocket + HTML |
| TCP客户端 | localhost:8080 | 运行 `go run client.go` |

## 命令

- `/users` - 查看在线用户
- `/rooms` - 查看房间列表
- `/join 房间名` - 加入房间
- `/msg 用户名 消息` - 私聊
- `/quit` - 退出

## 项目结构

```
tcp-chatroom/
├── server.go    # 服务器 (支持TCP+WebSocket)
├── client.go   # TCP客户端
├── README.md
└── go.mod
```