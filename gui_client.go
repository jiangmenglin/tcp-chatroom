package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var (
	serverAddr = "localhost:8080"
	name      string
	conn      net.Conn
)

func main() {
	a := app.New()
	w := a.NewWindow("TCP 聊天室")
	w.Resize(fyne.NewSize(700, 500))

	output := widget.NewTextArea()
	output.SetReadOnly(true)
	output.Wrapping = fyne.TextWrapWord

	input := widget.NewEntry()
	input.SetPlaceHolder("输入消息...")

	sendBtn := widget.NewButton("发送", func() {
		text := strings.TrimSpace(input.Text)
		if text == "" {
			return
		}
		input.SetText("")
		fmt.Fprintln(conn, text)
	})

	input.OnSubmitted = func(s string) {
		sendBtn.OnTapped()
	}

	roomEntry := widget.NewEntry()
	roomEntry.SetPlaceHolder("房间名...")

	joinBtn := widget.NewButton("加入房间", func() {
		roomName := strings.TrimSpace(roomEntry.Text)
		if roomName != "" {
			fmt.Fprintln(conn, "/join "+roomName)
			roomEntry.SetText("")
		}
	})

	msgLabel := widget.NewLabel("消息")

	inputArea := container.NewVBox(msgLabel, output)
	bottomArea := container.NewVBox(roomEntry, joinBtn, input, sendBtn)

	w.SetContent(container.NewVBox(inputArea, bottomArea))

	go connectAndRun(output)

	w.ShowAndRun()
}

func connectAndRun(output *widget.TextArea) {
	var err error
	conn, err = net.Dial("tcp", serverAddr)
	if err != nil {
		widget.NewErrorDialog(fyne.CurrentApp().Driver().RootWin(), "错误", "无法连接服务器")
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Println("已连接!")

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			os.Exit(0)
		}
		if strings.HasPrefix(line, "请输入你的昵称") {
			break
		}
	}

	if name == "" {
		name = "游客"
	}
	fmt.Fprintln(conn, name)

	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fmt.Fprintln(conn, "/ping")
			}
		}
	}()

	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		output.SetText(output.Text + msg)
	}
}