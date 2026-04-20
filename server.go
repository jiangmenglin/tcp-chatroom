package main

import (
	"bufio"
	"context"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

var (
	addr     = ":8080"
	tcpAddr  = ":8081"
)

type Client interface {
	GetName() string
	GetRoom() string
	SetRoom(string)
	GetMessages() chan string
}

type WSClient struct {
	conn     *websocket.Conn
	name     string
	messages chan string
	room     string
}

func (c *WSClient) GetName() string    { return c.name }
func (c *WSClient) GetRoom() string     { return c.room }
func (c *WSClient) SetRoom(r string)    { c.room = r }
func (c *WSClient) GetMessages() chan string { return c.messages }

type TCPClient struct {
	conn     net.Conn
	name     string
	messages chan string
	room     string
}

func (c *TCPClient) GetName() string    { return c.name }
func (c *TCPClient) GetRoom() string     { return c.room }
func (c *TCPClient) SetRoom(r string)    { c.room = r }
func (c *TCPClient) GetMessages() chan string { return c.messages }

type Message struct {
	content   string
	sender    string
	roomName  string
	timestamp string
}

type Room struct {
	name    string
	clients map[Client]bool
}

type Server struct {
	clients    map[Client]bool
	rooms      map[string]*Room
	broadcast  chan *Message
	register   chan Client
	unregister chan Client
	mu         sync.Mutex
}

func NewServer() *Server {
	return &Server{
		clients:    make(map[Client]bool),
		rooms:      make(map[string]*Room),
		broadcast:  make(chan *Message, 100),
		register:   make(chan Client),
		unregister: make(chan Client),
	}
}

func (s *Server) getOrCreateRoom(name string) *Room {
	if name == "" {
		name = "大厅"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if room, ok := s.rooms[name]; ok {
		return room
	}
	room := &Room{name: name, clients: make(map[Client]bool)}
	s.rooms[name] = room
	return room
}

func (s *Server) Run() {
	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client] = true
			count := len(s.clients)
			s.mu.Unlock()
			client.GetMessages() <- fmt.Sprintf("欢迎 %s 加入聊天室！当前在线人数: %d\n", client.GetName(), count)
			s.broadcast <- &Message{content: fmt.Sprintf("%s 加入了聊天室", client.GetName()), sender: "系统", roomName: client.GetRoom(), timestamp: time.Now().Format("15:04:05")}

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				if room, ok := s.rooms[client.GetRoom()]; ok {
					delete(room.clients, client)
					if len(room.clients) == 0 && room.name != "大厅" {
						delete(s.rooms, room.name)
					}
				}
				close(client.GetMessages())
				s.mu.Unlock()
				s.broadcast <- &Message{content: fmt.Sprintf("%s 离开了聊天室", client.GetName()), sender: "系统", roomName: client.GetRoom(), timestamp: time.Now().Format("15:04:05")}
			} else {
				s.mu.Unlock()
			}

		case msg := <-s.broadcast:
			s.mu.Lock()
			room := s.rooms[msg.roomName]
			if room != nil {
				for client := range room.clients {
					select {
					case client.GetMessages() <- formatMessage(msg):
					default:
					}
				}
			}
			s.mu.Unlock()
		}
	}
}

func formatMessage(msg *Message) string {
	if msg.sender == "系统" {
		return fmt.Sprintf("[%s] [系统] %s\n", msg.timestamp, msg.content)
	}
	return fmt.Sprintf("[%s] %s: %s\n", msg.timestamp, msg.sender, msg.content)
}

func (s *Server) sendTo(client Client, msg string) {
	select {
	case client.GetMessages() <- msg:
	default:
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println("WebSocket accept error:", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	client := &WSClient{
		conn:     conn,
		name:     "游客",
		messages: make(chan string, 50),
		room:     "大厅",
	}
	room := s.getOrCreateRoom("大厅")
	s.mu.Lock()
	room.clients[client] = true
	s.mu.Unlock()

	name := client.waitForName()
	if name != "" {
		client.name = name
	}

	s.register <- client

	go s.wsWritePump(client)
	s.wsReadPump(client)
}

func (c *WSClient) waitForName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, msg, err := c.conn.Read(ctx)
	if err != nil {
		return ""
	}
	return string(msg)
}

func (s *Server) wsReadPump(c *WSClient) {
	defer func() {
		s.unregister <- c
	}()

	ctx := context.Background()
	for {
		_, msg, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		text := string(msg)
		if text == "/users" {
			s.mu.Lock()
			c.messages <- fmt.Sprintf("在线用户 (%d):\n", len(s.clients))
			for cl := range s.clients {
				c.messages <- fmt.Sprintf("- %s\n", cl.GetName())
			}
			s.mu.Unlock()
			continue
		}
		if text == "/rooms" {
			s.mu.Lock()
			c.messages <- fmt.Sprintf("房间 (%d):\n", len(s.rooms))
			for name, room := range s.rooms {
				c.messages <- fmt.Sprintf("- %s (%d人)\n", name, len(room.clients))
			}
			s.mu.Unlock()
			continue
		}
		if len(text) > 6 && text[:5] == "/join" {
			newRoom := text[6:]
			s.mu.Lock()
			if oldRoom, ok := s.rooms[c.room]; ok {
				delete(oldRoom.clients, c)
				if len(oldRoom.clients) == 0 && oldRoom.name != "大厅" {
					delete(s.rooms, oldRoom.name)
				}
			}
			newR := s.getOrCreateRoom(newRoom)
			newR.clients[c] = true
			c.room = newRoom
			s.mu.Unlock()
			c.messages <- fmt.Sprintf("已切换到房间: %s\n", newRoom)
			continue
		}
		if len(text) > 5 && text[:4] == "/msg" {
			parts := splitN(text, " ", 3)
			if len(parts) >= 3 {
				targetName := parts[1]
				privateMsg := parts[2]
				s.mu.Lock()
				for cl := range s.clients {
					if cl.GetName() == targetName {
						timestamp := time.Now().Format("15:04:05")
						select {
						case cl.GetMessages() <- fmt.Sprintf("[%s] [%s] 私聊: %s\n", timestamp, c.name, privateMsg):
						default:
						}
					}
				}
				s.mu.Unlock()
			}
			continue
		}
		if text == "" {
			continue
		}
		s.broadcast <- &Message{content: text, sender: c.name, roomName: c.room, timestamp: time.Now().Format("15:04:05")}
	}
}

func (s *Server) wsWritePump(c *WSClient) {
	ctx := context.Background()
	for msg := range c.messages {
		err := c.conn.Write(ctx, websocket.MessageText, []byte(msg))
		if err != nil {
			return
		}
	}
}

// --- TCP handler ---

func (s *Server) handleTCPConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	fmt.Fprintln(conn, "请输入你的昵称: ")
	nameBytes, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	name := strings.TrimSpace(nameBytes)
	if name == "" {
		name = "游客"
	}
	if len(name) > 20 {
		name = name[:20]
	}

	tc := &TCPClient{
		conn:     conn,
		name:     name,
		messages: make(chan string, 50),
		room:     "大厅",
	}

	room := s.getOrCreateRoom("大厅")
	s.mu.Lock()
	room.clients[tc] = true
	s.mu.Unlock()

	s.register <- tc

	go s.tcpWritePump(tc)
	s.tcpReadPump(tc)
}

func (s *Server) tcpReadPump(tc *TCPClient) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, tc)
		if room, ok := s.rooms[tc.room]; ok {
			delete(room.clients, tc)
		}
		s.mu.Unlock()
		close(tc.messages)
		s.unregister <- tc
	}()

	reader := bufio.NewReader(tc.conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		if text == "/users" {
			s.mu.Lock()
			tc.messages <- fmt.Sprintf("在线用户 (%d):\n", len(s.clients))
			for cl := range s.clients {
				tc.messages <- fmt.Sprintf("- %s\n", cl.GetName())
			}
			s.mu.Unlock()
			continue
		}
		if text == "/rooms" {
			s.mu.Lock()
			tc.messages <- fmt.Sprintf("房间 (%d):\n", len(s.rooms))
			for name, room := range s.rooms {
				tc.messages <- fmt.Sprintf("- %s (%d人)\n", name, len(room.clients))
			}
			s.mu.Unlock()
			continue
		}
		if strings.HasPrefix(text, "/join ") {
			newRoom := strings.TrimSpace(text[6:])
			if newRoom == "" {
				continue
			}
			s.mu.Lock()
			if oldRoom, ok := s.rooms[tc.room]; ok {
				delete(oldRoom.clients, tc)
				if len(oldRoom.clients) == 0 && oldRoom.name != "大厅" {
					delete(s.rooms, oldRoom.name)
				}
			}
			newR := s.getOrCreateRoom(newRoom)
			newR.clients[tc] = true
			tc.room = newRoom
			s.mu.Unlock()
			tc.messages <- fmt.Sprintf("已切换到房间: %s\n", newRoom)
			continue
		}
		if strings.HasPrefix(text, "/msg ") {
			parts := splitN(text, " ", 3)
			if len(parts) >= 3 {
				targetName := parts[1]
				privateMsg := parts[2]
				s.mu.Lock()
				for cl := range s.clients {
					if cl.GetName() == targetName {
						timestamp := time.Now().Format("15:04:05")
						select {
						case cl.GetMessages() <- fmt.Sprintf("[%s] [%s] 私聊: %s\n", timestamp, tc.name, privateMsg):
						default:
						}
					}
				}
				s.mu.Unlock()
			}
			continue
		}
		if text == "/quit" {
			return
		}
		if text == "/ping" {
			continue
		}
		s.broadcast <- &Message{content: text, sender: tc.name, roomName: tc.room, timestamp: time.Now().Format("15:04:05")}
	}
}

func (s *Server) tcpWritePump(tc *TCPClient) {
	for msg := range tc.messages {
		_, err := fmt.Fprint(tc.conn, msg)
		if err != nil {
			return
		}
	}
}

func splitN(s string, sep string, n int) []string {
	result := []string{}
	start := 0
	for i := 0; i < n-1; i++ {
		idx := -1
		for j := start; j < len(s); j++ {
			if s[j] == ' ' {
				idx = j
				break
			}
		}
		if idx == -1 {
			break
		}
		result = append(result, s[start:idx])
		start = idx + 1
	}
	result = append(result, s[start:])
	return result
}

var htmlTemplate = `<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>TCP聊天室</title>
	<style>
		* { box-sizing: border-box; margin: 0; padding: 0; }
		body { font-family: Arial, sans-serif; background: #f5f5f5; height: 100vh; display: flex; flex-direction: column; }
		#login { position: absolute; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.8); display: flex; justify-content: center; align-items: center; z-index: 100; }
		#login-box { background: white; padding: 30px; border-radius: 8px; text-align: center; }
		#login-box h2 { margin-bottom: 20px; }
		#login-box input { padding: 10px; font-size: 16px; width: 200px; margin-bottom: 10px; }
		#login-box button { padding: 10px 30px; font-size: 16px; background: #4CAF50; color: white; border: none; cursor: pointer; border-radius: 4px; }
		#main { display: none; height: 100vh; flex-direction: column; }
		#header { background: #4CAF50; color: white; padding: 15px; display: flex; justify-content: space-between; align-items: center; }
		#content { flex: 1; display: flex; overflow: hidden; }
		#sidebar { width: 200px; background: white; border-right: 1px solid #ddd; overflow-y: auto; padding: 10px; }
		#sidebar h3 { font-size: 14px; color: #666; margin-bottom: 10px; }
		#sidebar ul { list-style: none; }
		#sidebar li { padding: 5px; cursor: pointer; }
		#sidebar li:hover { background: #f0f0f0; }
		#chat { flex: 1; display: flex; flex-direction: column; }
		#messages { flex: 1; overflow-y: auto; padding: 10px; background: white; }
		#messages div { margin-bottom: 8px; padding: 8px; background: #f0f0f0; border-radius: 4px; }
		#messages div.system { background: #e8f5e9; color: #2e7d32; }
		#messages div.private { background: #fff3e0; }
		#input-area { padding: 10px; background: white; border-top: 1px solid #ddd; display: flex; gap: 10px; }
		#input-area input { flex: 1; padding: 10px; font-size: 16px; border: 1px solid #ddd; border-radius: 4px; }
		#input-area button { padding: 10px 20px; background: #4CAF50; color: white; border: none; cursor: pointer; border-radius: 4px; }
		.room-info { font-size: 12px; color: #999; }
	</style>
</head>
<body>
	<div id="login">
		<div id="login-box">
			<h2>TCP 聊天室</h2>
			<input type="text" id="username" placeholder="输入昵称" maxlength="20">
			<br><br>
			<button onclick="join()">进入</button>
		</div>
	</div>
	<div id="main">
		<div id="header">
			<div>
				<h2>TCP 聊天室</h2>
				<span class="room-info" id="room-name">大厅</span>
			</div>
			<div>
				<span id="user-count">0 人在线</span>
			</div>
		</div>
		<div id="content">
			<div id="sidebar">
				<h3>在线用户</h3>
				<ul id="user-list"></ul>
				<br>
				<h3>房间</h3>
				<input type="text" id="room-input" placeholder="房间名" style="width:100%;padding:5px;margin-bottom:5px;">
				<button onclick="joinRoom()" style="padding:5px 10px;background:#2199F3;color:white;border:none;cursor:pointer;border-radius:3px;">加入</button>
				<ul id="room-list" style="margin-top:10px;"></ul>
			</div>
			<div id="chat">
				<div id="messages"></div>
				<div id="input-area">
					<input type="text" id="msg-input" placeholder="输入消息..." onkeypress="if(event.key==='Enter')send()">
					<button onclick="send()">发送</button>
				</div>
			</div>
		</div>
	</div>
	<script>
		let ws;
		let myName = '';
		
		function join() {
			const name = document.getElementById('username').value.trim();
			if (!name) { alert('请输入昵称'); return; }
			myName = name;
			connect();
		}
		
		function connect() {
			ws = new WebSocket('ws://' + location.host + '/ws');
			ws.onopen = () => {
				ws.send(myName);
				document.getElementById('login').style.display = 'none';
				document.getElementById('main').style.display = 'flex';
			};
			ws.onmessage = (e) => {
				const msg = e.data;
				if (msg.startsWith('在线用户')) {
					parseUsers(msg);
				} else if (msg.startsWith('房间 (')) {
					parseRooms(msg);
				} else if (msg.startsWith('[系统]')) {
					addMessage(msg, 'system');
				} else if (msg.includes('私聊')) {
					addMessage(msg, 'private');
				} else {
					addMessage(msg, '');
				}
			};
			ws.onclose = () => {
				addMessage('连接已断开，3秒后重连...', 'system');
				setTimeout(connect, 3000);
			};
		}
		
		function addMessage(msg, type) {
			const div = document.getElementById('messages');
			const p = document.createElement('div');
			p.textContent = msg;
			if (type) p.className = type;
			div.appendChild(p);
			div.scrollTop = div.scrollHeight;
		}
		
		function parseUsers(msg) {
			const lines = msg.split('\n');
			const ul = document.getElementById('user-list');
			ul.innerHTML = '';
			let count = 0;
			lines.forEach(line => {
				if (line.startsWith('- ')) {
					const li = document.createElement('li');
					li.textContent = line.substring(2);
					ul.appendChild(li);
					count++;
				}
			});
			document.getElementById('user-count').textContent = count + ' 人在线';
		}
		
		function parseRooms(msg) {
			const lines = msg.split('\n');
			const ul = document.getElementById('room-list');
			ul.innerHTML = '';
			lines.forEach(line => {
				if (line.startsWith('- ')) {
					const li = document.createElement('li');
					li.textContent = line;
					ul.appendChild(li);
				}
			});
		}
		
		function send() {
			const input = document.getElementById('msg-input');
			const text = input.value.trim();
			if (!text) return;
			input.value = '';
			ws.send(text);
			if (text === '/users') ws.send('\n');
			if (text === '/rooms') ws.send('\n');
		}
		
		function joinRoom() {
			const name = document.getElementById('room-input').value.trim();
			if (!name) return;
			ws.send('/join ' + name);
			document.getElementById('room-name').textContent = name;
		}
	</script>
</body>
</html>`

func main() {
	server := NewServer()
	go server.Run()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, _ := template.New("index").Parse(htmlTemplate)
		t.Execute(w, nil)
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		server.handleWebSocket(w, r)
	})

	go func() {
		fmt.Println("HTTP/WebSocket 服务器启动: http://localhost" + addr)
		log.Fatal(http.ListenAndServe(addr, nil))
	}()

	go func() {
		l, err := net.Listen("tcp", tcpAddr)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("TCP 服务器启动: localhost" + tcpAddr)
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Println("accept error:", err)
				continue
			}
			go server.handleTCPConn(conn)
		}
	}()

	fmt.Println("========================================")
	fmt.Println("  聊天室服务器已启动")
	fmt.Println("  HTTP/WebSocket: http://localhost" + addr)
	fmt.Println("  TCP: localhost" + tcpAddr)
	fmt.Println("========================================")

	select {}
}