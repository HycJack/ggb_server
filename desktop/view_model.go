package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/HycJack/MujicaUI/account"
	"github.com/HycJack/MujicaUI/chat"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/markdown"
)

// app 是全部应用状态；view 只读它，事件在构建时写入，后台 goroutine 一律经 win.Update 改它。
type app struct {
	win *mygo.Window

	// 认证
	s        settings
	logged   bool
	login    account.LoginFormState
	loginURL string
	loginErr string
	busy     bool // 登录/请求进行中

	// 会话
	convs    []conversation
	convSel  chat.ConversationListState
	convID   uint // 当前打开的会话
	loadingC bool

	// 消息
	msgs   []*uiMsg
	list   chat.MessageListState
	input  string
	nextID int

	// 共享的 markdown 渲染配置（全部消息共用一份）
	md markdown.Options

	api *Client

	// GeoGebra 画板
	ggbView      *mygo.WebView
	showBoard    bool
	boardReady   bool
	boardBusy    bool
	boardApplied int
	boardErrors  []string
	boardSeq     int
}

func newApp() *app {
	a := &app{}
	a.s = loadSettings()
	a.loginURL = a.s.ServerURL
	a.logged = a.s.Token != ""
	a.api = NewClient(a.s.ServerURL)
	a.showBoard = true
	return a
}

func (a *app) setClient(url string) {
	a.s.ServerURL = url
	a.api = NewClient(url)
}

// ---- 动作 ----

func (a *app) doLogin(accountName, password string, _ bool) {
	if a.busy {
		return
	}
	a.busy = true
	a.loginErr = ""
	run := func() (string, loginUserInfo, error) { return a.api.Login(accountName, password) }
	finish := func(tok string, u loginUserInfo, err error) {
		a.busy = false
		if err != nil {
			a.loginErr = err.Error()
			a.login.EndSubmit(err.Error())
			return
		}
		a.s.Token, a.s.Username, a.s.UserID = tok, u.Username, u.UserId
		saveSettings(a.s)
		a.logged = true
		a.login.EndSubmit("")
		a.refreshConversations()
	}
	if a.win == nil { // headless：同步落定，保证测试确定性
		tok, u, err := run()
		finish(tok, u, err)
		return
	}
	go func() {
		tok, u, err := run()
		a.win.Update(func() { finish(tok, u, err) })
	}()
}

func (a *app) logout() {
	a.logged = false
	a.s.Token = ""
	saveSettings(a.s)
	a.msgs = nil
	a.convs = nil
	a.convID = 0
	a.convSel.Selected = ""
}

func (a *app) refreshConversations() {
	if a.s.Token == "" {
		return
	}
	a.loadingC = true
	run := func() ([]conversation, error) { return a.api.Conversations(a.s.Token) }
	done := func(cs []conversation, err error) {
		a.loadingC = false
		if err == nil {
			a.convs = cs
		}
	}
	if a.win == nil {
		cs, err := run()
		done(cs, err)
		return
	}
	go func() {
		cs, err := run()
		a.win.Update(func() { done(cs, err) })
	}()
}

// selectConversation 在用户点选会话后载入历史。
func (a *app) selectConversation(id uint) {
	if id == 0 || id == a.convID {
		return
	}
	a.convID = id
	a.msgs = nil
	a.loadingC = true
	run := func() ([]histMessage, error) { return a.api.History(a.s.Token, id) }
	done := func(hist []histMessage, err error) {
		a.loadingC = false
		if err == nil {
			a.msgs = toUiMsgs(hist)
		}
	}
	if a.win == nil {
		hist, err := run()
		done(hist, err)
		return
	}
	go func() {
		hist, err := run()
		a.win.Update(func() { done(hist, err) })
	}()
}

func (a *app) newConversation() {
	a.convID = 0
	a.convSel.Selected = ""
	a.msgs = nil
}

// send 发送一条消息：追加用户气泡与占位回复，再起 goroutine 流式填充。
func (a *app) send(text string) {
	if a.streaming() || text == "" {
		return
	}
	a.nextID++
	a.msgs = append(a.msgs, &uiMsg{id: "u" + strconv.Itoa(a.nextID), role: chat.MessageUser, text: text, done: true})
	a.nextID++
	reply := &uiMsg{id: "a" + strconv.Itoa(a.nextID), role: chat.MessageAssistant, open: true, started: time.Now()}
	a.msgs = append(a.msgs, reply)
	a.input = ""

	if a.win == nil { // headless：同步给出确定性的占位回复
		reply.text = "（离线演示）这里会流式显示模型输出的数学元素提取结果。"
		reply.ggb = "A = (1, 2)\nB = (4, 6)\nf(x) = sin(x)"
		reply.html = "<!DOCTYPE html><html><head></head><body>demo</body></html>"
		reply.done = true
		return
	}
	go a.streamChat(text, reply)
}

func (a *app) streaming() bool {
	return len(a.msgs) > 0 && !a.msgs[len(a.msgs)-1].done
}

// streamChat 确保会话存在、发起 SSE 请求并把事件应用到 reply 上。
func (a *app) streamChat(text string, reply *uiMsg) {
	if a.convID == 0 {
		id, err := a.api.CreateConversation(a.s.Token, truncTitle(text))
		if err != nil {
			a.fail(reply, "创建会话失败："+err.Error())
			return
		}
		a.convID = id
		a.win.Update(func() {
			a.convs = append([]conversation{{ID: id, Title: truncTitle(text), UpdatedAt: time.Now()}}, a.convs...)
			a.convSel.Selected = fmt.Sprintf("%d", id)
		})
	}
	sessionID := a.convID
	apply := func(ev SSEEvent) {
		a.win.Update(func() { a.applyEvent(reply, ev) })
	}
	if err := a.api.Chat(a.s.Token, sessionID, text, apply); err != nil {
		a.win.Update(func() { a.fail(reply, err.Error()) })
		return
	}
	a.win.Update(func() { reply.done = true })
}

func (a *app) fail(reply *uiMsg, msg string) {
	reply.errText = msg
	reply.done = true
}

// applyEvent 把一个 SSE 事件合并进占位回复。
func (a *app) applyEvent(reply *uiMsg, ev SSEEvent) {
	if reply.done {
		return
	}
	switch ev.Type {
	case "reasoning":
		reply.thinking += ev.Content
		reply.step = stepLabel(ev.Step)
	case "element":
		reply.text += ev.Content
		reply.step = stepLabel(ev.Step)
	case "ggb_commands":
		reply.ggb = ev.Content
		reply.step = stepLabel(ev.Step)
		// 指令一到就推给画板，不等整条回复结束
		a.sendToBoard(false, "", ev.Content)
	case "html_code":
		reply.html = ev.Content
		reply.step = stepLabel(ev.Step)
	case "failed_text", "error":
		reply.errText = firstNonEmpty(ev.Content, "处理失败，请换个问题试试")
		reply.done = true
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// openArtifact 把生成的 HTML 存到临时目录并用系统浏览器打开。
func (a *app) openArtifact(html string) {
	dir := filepath.Join(os.TempDir(), "ggb-desktop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	p := filepath.Join(dir, fmt.Sprintf("artifact-%d.html", time.Now().UnixNano()))
	if err := os.WriteFile(p, []byte(html), 0o644); err != nil {
		return
	}
	cmd := "open"
	if runtime.GOOS != "darwin" {
		cmd = "xdg-open"
	}
	exec.Command(cmd, p).Start()
}
