//go:build windows

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// --- List Models ---

type StringModel struct {
	walk.ListModelBase
	items []string
}

func NewStringModel(items []string) *StringModel {
	return &StringModel{items: items}
}

func (m *StringModel) ItemCount() int {
	return len(m.items)
}

func (m *StringModel) Value(index int) interface{} {
	return m.items[index]
}

func (m *StringModel) Set(items []string) {
	m.items = items
	m.PublishItemsReset()
}

// --- Chat Client ---

type ChatClient struct {
	conn       net.Conn
	serverAddr string
	nickName   string
	mu         sync.Mutex

	msgView     *walk.TextEdit
	inputEdit   *walk.LineEdit
	userList    *walk.ListBox
	roomList    *walk.ListBox
	roomInput   *walk.LineEdit
	statusLabel *walk.Label

	userModel *StringModel
	roomModel *StringModel

	chatMode      string // "room" or "private"
	privateTarget string
}

func main() {
	client := &ChatClient{
		serverAddr: "localhost:8081",
		chatMode:   "room",
	}

	if !client.showLoginDialog() {
		return
	}

	client.runMainWindow()
}

// --- Login Dialog ---

func (c *ChatClient) showLoginDialog() bool {
	var dlg *walk.Dialog
	var nickEdit, addrEdit *walk.LineEdit

	result := false

	Dialog{
		AssignTo: &dlg,
		Title:    "TCP 聊天室 - 登录",
		MinSize:  Size{Width: 320, Height: 180},
		Layout:   VBox{},
		Children: []Widget{
			Composite{
				Layout: Grid{Columns: 2},
				Children: []Widget{
					Label{Text: "昵称:"},
					LineEdit{
						AssignTo:  &nickEdit,
						Text:      "游客",
						MaxLength: 20,
					},
					Label{Text: "服务器:"},
					LineEdit{
						AssignTo: &addrEdit,
						Text:     "localhost:8081",
					},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{
						Text: "连接",
						OnClicked: func() {
							nick := strings.TrimSpace(nickEdit.Text())
							addr := strings.TrimSpace(addrEdit.Text())
							if nick == "" {
								walk.MsgBox(dlg, "提示", "请输入昵称", walk.MsgBoxIconWarning)
								return
							}
							if addr == "" {
								walk.MsgBox(dlg, "提示", "请输入服务器地址", walk.MsgBoxIconWarning)
								return
							}
							conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
							if err != nil {
								walk.MsgBox(dlg, "连接失败", "无法连接到 "+addr, walk.MsgBoxIconError)
								return
							}
							c.conn = conn
							c.nickName = nick
							c.serverAddr = addr
							result = true
							dlg.Accept()
						},
					},
					PushButton{
						Text: "取消",
						OnClicked: func() {
							dlg.Cancel()
						},
					},
				},
			},
		},
	}.Run(nil)

	return result
}

// --- Main Window ---

func (c *ChatClient) runMainWindow() {
	var mainWindow *walk.MainWindow

	c.userModel = NewStringModel(nil)
	c.roomModel = NewStringModel(nil)

	if err := (MainWindow{
		AssignTo: &mainWindow,
		Title:    "TCP 聊天室 - " + c.nickName,
		MinSize:  Size{Width: 750, Height: 500},
		Layout:   VBox{},
		MenuItems: []MenuItem{
			Menu{
				Text: "操作",
				Items: []MenuItem{
					Action{
						Text: "查看在线用户 (/users)",
						OnTriggered: func() {
							c.send("/users")
						},
					},
					Action{
						Text: "查看房间列表 (/rooms)",
						OnTriggered: func() {
							c.send("/rooms")
						},
					},
					Separator{},
					Action{
						Text: "退出",
						OnTriggered: func() {
							c.send("/quit")
							mainWindow.Close()
						},
					},
				},
			},
		},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{
						AssignTo: &c.statusLabel,
						Text:     "状态: 已连接 - 大厅",
					},
					HSpacer{},
					Label{Text: "双击用户私聊 | 双击房间切换"},
				},
			},
			HSplitter{
				Children: []Widget{
					Composite{
						MinSize: Size{Width: 180},
						Layout:  VBox{},
						Children: []Widget{
							Label{Text: "在线用户"},
							ListBox{
								AssignTo: &c.userList,
								Model:    c.userModel,
								OnItemActivated: func() {
									idx := c.userList.CurrentIndex()
									if idx >= 0 && idx < c.userModel.ItemCount() {
										name := c.userModel.items[idx]
										if name == c.nickName {
											return
										}
										c.chatMode = "private"
										c.privateTarget = name
										c.statusLabel.SetText("状态: 私聊 -> " + name)
									}
								},
							},
							Label{Text: "房间"},
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									LineEdit{
										AssignTo: &c.roomInput,
									},
									PushButton{
										Text: "加入",
										OnClicked: func() {
											room := strings.TrimSpace(c.roomInput.Text())
											if room != "" {
												c.send("/join " + room)
												c.roomInput.SetText("")
											}
										},
									},
								},
							},
							ListBox{
								AssignTo: &c.roomList,
								Model:    c.roomModel,
								OnItemActivated: func() {
									idx := c.roomList.CurrentIndex()
									if idx >= 0 && idx < c.roomModel.ItemCount() {
										room := c.roomModel.items[idx]
										room = strings.SplitN(room, " (", 2)[0]
										c.send("/join " + room)
										c.chatMode = "room"
										c.statusLabel.SetText("状态: 已连接 - " + room)
									}
								},
							},
						},
					},
					Composite{
						Layout: VBox{},
						Children: []Widget{
							TextEdit{
								AssignTo: &c.msgView,
								ReadOnly: true,
								VScroll:  true,
								Text:     "欢迎使用 TCP 聊天室!\r\n",
							},
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									LineEdit{
										AssignTo: &c.inputEdit,
										OnKeyDown: func(key walk.Key) {
											if key == walk.KeyReturn {
												c.doSend()
											}
										},
									},
									PushButton{
										Text: "发送",
										OnClicked: func() {
											c.doSend()
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}.Create()); err != nil {
		walk.MsgBox(nil, "错误", "创建窗口失败: "+err.Error(), walk.MsgBoxIconError)
		return
	}

	go c.handshake()
	go c.readLoop()
	go c.heartbeat()

	mainWindow.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		c.send("/quit")
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.mu.Unlock()
	})

	mainWindow.Run()
}

// --- Network ---

func (c *ChatClient) handshake() {
	reader := bufio.NewReader(c.conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			c.appendMsg("[系统] 握手失败，连接断开")
			return
		}
		if strings.HasPrefix(line, "请输入你的昵称") {
			fmt.Fprintln(c.conn, c.nickName)
			break
		}
	}
}

func (c *ChatClient) readLoop() {
	reader := bufio.NewReader(c.conn)
	c.readFrom(reader)
}

func (c *ChatClient) readFrom(reader *bufio.Reader) {
	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			c.appendMsg("[系统] 连接已断开，尝试重连...")
			go c.reconnect()
			return
		}
		msg = strings.TrimRight(msg, "\r\n")
		if msg == "" {
			continue
		}
		c.handleMessage(msg)
	}
}

func (c *ChatClient) handleMessage(msg string) {
	if strings.HasPrefix(msg, "在线用户") || strings.HasPrefix(msg, "房间") {
		c.parseList(msg)
		return
	}
	if strings.Contains(msg, "已切换到房间") {
		parts := strings.SplitN(msg, ":", 2)
		if len(parts) == 2 {
			room := strings.TrimSpace(parts[1])
			c.chatMode = "room"
			c.statusLabel.SetText("状态: 已连接 - " + room)
		}
	}
	if strings.Contains(msg, "加入了聊天室") || strings.Contains(msg, "离开了聊天室") {
		c.send("/users")
	}
	c.appendMsg(msg)
}

func (c *ChatClient) parseList(msg string) {
	lines := strings.Split(msg, "\n")
	if strings.HasPrefix(msg, "在线用户") {
		var users []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				users = append(users, line[2:])
			}
		}
		c.userModel.Set(users)
	} else if strings.HasPrefix(msg, "房间") {
		var rooms []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				rooms = append(rooms, line[2:])
			}
		}
		c.roomModel.Set(rooms)
	}
}

func (c *ChatClient) doSend() {
	text := strings.TrimSpace(c.inputEdit.Text())
	if text == "" {
		return
	}
	c.inputEdit.SetText("")

	if c.chatMode == "private" && c.privateTarget != "" && !strings.HasPrefix(text, "/") {
		c.send("/msg " + c.privateTarget + " " + text)
		return
	}
	c.send(text)
}

func (c *ChatClient) send(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		fmt.Fprintln(c.conn, text)
	}
}

func (c *ChatClient) appendMsg(msg string) {
	c.msgView.Synchronize(func() {
		text := c.msgView.Text()
		if len(text) > 0 && !strings.HasSuffix(text, "\r\n") {
			text += "\r\n"
		}
		c.msgView.SetText(text + msg)
		c.msgView.SendMessage(0x0115, 7, 0) // WM_VSCROLL, SB_BOTTOM
	})
}

func (c *ChatClient) heartbeat() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		if c.conn == nil {
			c.mu.Unlock()
			return
		}
		fmt.Fprintln(c.conn, "/ping")
		c.mu.Unlock()
	}
}

func (c *ChatClient) reconnect() {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	for i := 0; i < 10; i++ {
		time.Sleep(3 * time.Second)
		conn, err := net.DialTimeout("tcp", c.serverAddr, 5*time.Second)
		if err != nil {
			c.appendMsg(fmt.Sprintf("[系统] 重连失败，%d/10...", i+1))
			continue
		}
		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()

		c.appendMsg("[系统] 重连成功!")

		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			if strings.HasPrefix(line, "请输入你的昵称") {
				fmt.Fprintln(conn, c.nickName)
				break
			}
		}

		go c.readFrom(reader)
		go c.heartbeat()
		return
	}
	c.appendMsg("[系统] 重连失败，请重启客户端")
	os.Exit(1)
}
