package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// app 是全部应用状态；view 只读它，事件在构建时写入，后台 goroutine 一律经 win.Update 改它。
type app struct {
	win *mygo.Window

	s settings // OpenAI 端点配置（持久化）

	// 会话（本地保存）
	convs    []conversation
	msgsOf   map[uint][]*uiMsg
	convID   uint
	nextConv uint

	// 消息
	list   ui.ListState
	input  string
	nextID int

	// 附件（本地图片，base64 直发视觉模型）
	attachPath   string
	attachData   string // data:image/...;base64,...
	attachBitmap *ui.Bitmap
	attachSize   int64
	attachBusy   bool
	attachErr    string

	// 布局（宽度由原生 ui.Split 管理）
	sidebarWidth float32
	showSidebar  bool
	chatWidth    float32
	showBoard    bool
	showSettings bool

	// GeoGebra 画板
	ggbView      *mygo.WebView
	boardReady   bool
	boardBusy    bool
	boardApplied int
	boardErrors  []string
	boardSeq     int

	md  markdown.Options
	api *OpenAIClient
}

func newApp() *app {
	a := &app{}
	a.s = loadSettings()
	a.showSidebar = true
	a.showBoard = true
	a.sidebarWidth = 260
	a.chatWidth = 500
	a.msgsOf = map[uint][]*uiMsg{}
	if msgs, convs, err := loadHistory(); err == nil && len(convs) > 0 {
		a.msgsOf = msgs
		a.convs = convs
		a.nextConv = maxID(convs)
	}
	a.rebuildClient()
	return a
}

func maxID(convs []conversation) uint {
	var max uint
	for _, cv := range convs {
		if cv.ID > max {
			max = cv.ID
		}
	}
	return max
}

func (a *app) rebuildClient() {
	a.api = NewOpenAIClient(a.s.APIBase, a.s.APIKey)
}

func (a *app) configured() bool {
	return a.s.APIBase != "" && a.s.Model != ""
}

func (a *app) saveAll() {
	saveSettings(a.s)
	saveHistory(a.msgsOf, a.convs)
}

// ---- 会话 ----

func (a *app) currentMsgs() []*uiMsg { return a.msgsOf[a.convID] }

func (a *app) currentTitle() string {
	for _, cv := range a.convs {
		if cv.ID == a.convID {
			return cv.Title
		}
	}
	return "新对话"
}

func (a *app) newConversation() {
	a.convID = 0
	a.input = ""
}

func (a *app) selectConversation(id uint) {
	if id == 0 || id == a.convID {
		return
	}
	a.convID = id
	a.input = ""
	if _, ok := a.msgsOf[id]; !ok {
		a.msgsOf[id] = nil
	}
}

func (a *app) deleteConversation(id uint) {
	delete(a.msgsOf, id)
	for i, cv := range a.convs {
		if cv.ID == id {
			a.convs = append(a.convs[:i], a.convs[i+1:]...)
			break
		}
	}
	if a.convID == id {
		a.convID = 0
	}
	a.saveAll()
}

// ---- 附件 ----

// pickImage 选本地图片：文件对话框在后台 goroutine 打开（绝不能在 UI
// 线程同步等 sheet），像素解码也在后台做纯 CPU 部分，Bitmap 只在主线程创建。
func (a *app) pickImage() {
	if a.win == nil || a.attachBusy {
		return
	}
	a.attachBusy = true
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win,
			Title:  "选择题目图片",
			Filters: []mygo.FileFilter{
				{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}},
			},
		})
		if err != nil || len(paths) == 0 {
			a.win.Update(func() { a.attachBusy = false })
			return
		}
		path := paths[0]
		a.win.Update(func() {
			a.attachPath = path
			a.attachData = ""
			a.attachBitmap = nil
			a.attachErr = ""
			a.attachSize = 0
		})

		data, readErr := os.ReadFile(path)
		mime := mimeOf(path)
		var dataURL string
		var img image.Image
		var decErr error
		if readErr == nil {
			dataURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
			img, _, decErr = image.Decode(bytes.NewReader(data)) // 纯 CPU，png/jpg/gif
		}
		a.win.Update(func() {
			a.attachBusy = false
			if readErr != nil {
				a.attachErr = readErr.Error()
				return
			}
			if decErr != nil {
				// webp 等原生不支持的格式：DecodeBitmap 兜底（必须在主线程）
				bm, bmErr := ui.DecodeBitmap(data)
				if bmErr != nil {
					a.attachErr = "图片解码失败：" + bmErr.Error()
					return
				}
				a.attachBitmap = bm
			} else {
				a.attachBitmap = ui.NewBitmap(img)
			}
			a.attachData = dataURL
			a.attachSize = int64(len(data))
		})
	}()
}

func mimeOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func (a *app) clearAttachment() {
	a.attachPath = ""
	a.attachData = ""
	a.attachBitmap = nil
	a.attachSize = 0
	a.attachBusy = false
	a.attachErr = ""
}

// ---- 发送与流式 ----

func (a *app) send(text string) {
	if a.streaming() || text == "" || !a.configured() {
		return
	}
	// 新会话：先落一个 ID
	if a.convID == 0 {
		a.nextConv++
		a.convID = a.nextConv
		a.convs = append(a.convs, conversation{ID: a.convID, Title: truncTitle(text)})
	}
	a.nextID++
	userMsg := &uiMsg{id: "u" + strconv.Itoa(a.nextID), role: roleUser, text: text, done: true}
	if a.attachData != "" {
		userMsg.dataURL = a.attachData
		userMsg.bitmap = a.attachBitmap
	}
	a.msgsOf[a.convID] = append(a.msgsOf[a.convID], userMsg)
	a.clearAttachment()
	a.nextID++
	reply := &uiMsg{id: "a" + strconv.Itoa(a.nextID), role: roleAssistant, open: true, started: time.Now(), step: "思考中"}
	a.msgsOf[a.convID] = append(a.msgsOf[a.convID], reply)
	a.input = ""

	if a.win == nil { // headless：同步落定，保证截图确定性
		reply.text = "好的，这道题的作图指令如下：\n\n```ggb\nf(x) = x^2\n```"
		reply.done = true
		a.applyGGBFences(reply)
		a.saveAll()
		return
	}
	go a.streamChat(text, reply)
}

func (a *app) streaming() bool {
	msgs := a.currentMsgs()
	return len(msgs) > 0 && !msgs[len(msgs)-1].done
}

// buildMessages 把当前会话整理成 OpenAI 消息数组（系统提示 + 截断后的历史 + 带图消息）。
func (a *app) buildMessages(userText, imageDataURL string) []OAMessage {
	var out []OAMessage
	if a.s.SystemPrompt != "" {
		out = append(out, textMessage("system", a.s.SystemPrompt))
	}
	msgs := a.currentMsgs()
	// 去掉刚 append 的用户消息与占位回复，从历史正文构建
	history := msgs
	if len(history) > 2 {
		history = history[:len(history)-2]
	}
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	for _, m := range history {
		if m.errText != "" || m.text == "" {
			continue
		}
		if m.role == roleUser {
			out = append(out, textMessage("user", m.text))
		} else {
			out = append(out, textMessage("assistant", m.text))
		}
	}
	if imageDataURL != "" {
		out = append(out, OAMessage{Role: "user", Content: []OAContentPart{
			{Type: "text", Text: userText},
			{Type: "image_url", ImageURL: &OAImageURL{URL: imageDataURL}},
		}})
	} else {
		out = append(out, textMessage("user", userText))
	}
	return out
}

// streamChat 发起流式请求并把增量应用到占位回复上。
func (a *app) streamChat(userText string, reply *uiMsg) {
	history := a.buildMessages(userText, a.replyDataImage(reply))
	apply := func(ev OAEvent) {
		a.win.Update(func() {
			if ev.Reasoning != "" {
				reply.thinking += ev.Reasoning
			}
			if ev.Content != "" {
				reply.text += ev.Content
				reply.step = ""
				a.applyGGBFences(reply) // 围栏一到就画板
			}
		})
	}
	if err := a.api.Chat(a.s.Model, history, apply); err != nil {
		a.win.Update(func() {
			reply.errText = err.Error()
			reply.done = true
			a.saveAll()
		})
		return
	}
	a.win.Update(func() {
		reply.done = true
		a.applyGGBFences(reply)
		a.applyHTMLFence(reply)
		a.saveAll()
	})
}

// replyDataImage 取占位回复对应的那条用户消息里的图片（倒数第二条）。
func (a *app) replyDataImage(reply *uiMsg) string {
	msgs := a.currentMsgs()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i] == reply {
			if i > 0 && msgs[i-1].role == roleUser {
				return msgs[i-1].dataURL
			}
			return ""
		}
	}
	return ""
}

// applyGGBFences 抽取回复里的 ```ggb 围栏，配置开启时推给画板。
func (a *app) applyGGBFences(reply *uiMsg) {
	cmds := extractGGBBlocks(reply.text)
	if len(cmds) == 0 {
		return
	}
	reply.ggb = strings.Join(cmds, "\n")
	if a.s.AutoBoard && a.ggbView != nil {
		a.sendToBoard(false, "", reply.ggb)
	} else if a.s.AutoBoard {
		a.boardApplied = len(cmds)
	}
}

// applyHTMLFence 抽取回复里的 ```html 围栏（整页 GeoGebra HTML 时出现），
// 存到消息上供「在浏览器打开」使用。
func (a *app) applyHTMLFence(reply *uiMsg) {
	blocks := extractFenced(reply.text, "html")
	if len(blocks) > 0 {
		reply.html = blocks[len(blocks)-1]
	}
}

func (a *app) fail(reply *uiMsg, msg string) {
	reply.errText = msg
	reply.done = true
}

// ---- 画板 ----

// sendToBoard 把一批指令发到画板；headless 下只记录状态供截图。
func (a *app) sendToBoard(reset bool, perspective, text string) {
	commands := splitCommands(text)
	if len(commands) == 0 && !reset && perspective == "" {
		return
	}
	a.boardSeq++
	msg := boardMsg{ID: "b" + strconv.Itoa(a.boardSeq), Perspective: perspective, Reset: reset, Commands: commands}
	a.boardBusy = a.ggbView != nil
	a.boardErrors = nil
	if a.ggbView == nil {
		a.boardApplied = len(commands) // headless：假装全部成功，保证测试确定性
		return
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	ggbCmdsEvent.Broadcast(string(payload))
}

// ---- 工具 ----

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

var _ = firstNonEmpty

// extractFenced 抽取指定语言的围栏代码块内容。
func extractFenced(text, lang string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	in := false
	var cur []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if !in {
			if strings.HasPrefix(lower, "```"+lang) {
				in = true
				cur = nil
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			in = false
			out = append(out, strings.Join(cur, "\n"))
			continue
		}
		cur = append(cur, line)
	}
	return out
}

func splitCommands(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		out = append(out, line)
	}
	return out
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
