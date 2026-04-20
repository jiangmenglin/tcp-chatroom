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
| 命令行客户端 | localhost:8080 | 运行 `go run client.go` |
| GUI客户端 | localhost:8080 | 运行 `gui_client.exe`（仅 Windows） |

### GUI 客户端

基于 [walk](https://github.com/lxn/walk) 的 Windows 原生界面客户端，功能包括：

- 登录对话框（自定义昵称和服务器地址）
- 在线用户列表（双击用户发起私聊）
- 房间列表（双击房间快速切换）
- 断线自动重连

## 命令

- `/users` - 查看在线用户
- `/rooms` - 查看房间列表
- `/join 房间名` - 加入房间
- `/msg 用户名 消息` - 私聊
- `/quit` - 退出

## 项目结构

```
tcp-chatroom/
├── server.go      # 服务器 (支持TCP+WebSocket)
├── client.go      # 命令行TCP客户端
├── gui_client.go  # Windows GUI客户端 (walk)
├── README.md
└── go.mod
```