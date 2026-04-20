package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	serverAddr = "localhost:8081"
	name      string
)

func main() {
	fmt.Print("连接服务器...")
	for {
		conn, err := net.Dial("tcp", serverAddr)
		if err == nil {
			fmt.Println("成功!")
			runClient(conn)
			fmt.Println("连接已断开，等待重连...")
		} else {
			fmt.Println("失败，3秒后重试...")
		}
		time.Sleep(3 * time.Second)
	}
}

func runClient(conn net.Conn) {
	defer conn.Close()

	fmt.Println("已连接到聊天室服务器!")
	fmt.Println("命令: /users 查看在线用户, /msg 用户名 消息 私聊, /quit 退出")
	fmt.Println("============================================")

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if strings.HasPrefix(line, "请输入你的昵称") {
			break
		}
	}

	fmt.Print("> ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		name = scanner.Text()
		name = strings.TrimSpace(name)
		if name == "" {
			name = "游客"
		}
		if len(name) > 20 {
			name = name[:20]
		}
		fmt.Fprintln(conn, name)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			msg, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			fmt.Print(msg)
			fmt.Print("> ")
		}
	}()

	go func() {
		defer wg.Done()
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		for scanner.Scan() {
			select {
			case <-ticker.C:
				fmt.Fprintln(conn, "/ping")
			default:
			}

			text := scanner.Text()
			if text == "" {
				fmt.Print("> ")
				continue
			}
			_, err := fmt.Fprintln(conn, text)
			if err != nil {
				return
			}
			if text == "/quit" || text == "/reconnect" {
				return
			}
		}
	}()

	wg.Wait()
}