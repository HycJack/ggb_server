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
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// app 是全部应用状态；view 只读它，事件在构建时写入，后台 goroutine 一律经 win.Update 改它。
type app struct {
	win *mygo.Window

	s     settings // OpenAI 端点配置（持久化）
	draft settings // 设置弹窗的草稿：只有点「保存」才写回 s

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
	ggbView          *mygo.WebView
	boardReady       bool
	boardBusy        bool
	boardApplied     int
	boardErrors      []string
	boardObjectCount int
	boardObjects     []string
	boardNote        string
	boardImageURL    string // 最近一次画板截图（data URL）
	boardBitmapCache *ui.Bitmap
	boardSeq         int

	boardMu      sync.Mutex
	boardPending map[string]chan boardResult

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
	a.boardPending = map[string]chan boardResult{}
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

// openSettings 打开设置弹窗：先把当前配置复制到草稿，
// 弹窗里的输入不会直接污染真实配置。
func (a *app) openSettings() {
	a.draft = a.s
	a.showSettings = true
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
	// 去掉刚 append 的用户消息与占位回复（末尾两条），只留更早的历史
	history := msgs
	if len(history) >= 2 {
		history = history[:len(history)-2]
	} else {
		history = nil
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

// mutate 把状态修改派发到 UI 线程；无窗口（测试）时同步执行。
func (a *app) mutate(f func()) {
	if a.win == nil {
		f()
		return
	}
	a.win.Update(f)
}

// maxToolRounds 限制一次回答里 ReAct 的最大轮数，防止死循环。
const maxToolRounds = 6

// streamChat 发起流式请求；开启工具调用时跑 ReAct 循环：
// 模型请求工具 → 在画板上同步执行 → 结果回传 → 模型继续，直到给出最终回答。
func (a *app) streamChat(userText string, reply *uiMsg) {
	messages := a.buildMessages(userText, a.replyDataImage(reply))
	apply := func(ev OAEvent) {
		a.mutate(func() {
			if ev.Reasoning != "" {
				reply.thinking += ev.Reasoning
			}
			if ev.Content != "" {
				reply.text += ev.Content
				reply.step = ""
			}
		})
	}

	useTools := a.s.UseTools && a.ggbView != nil
	var tools []map[string]any
	if useTools {
		tools = BoardTools
	}

	var res OAResult
	for round := 0; ; round++ {
		var err error
		res, err = a.api.chat(a.s.Model, messages, tools, apply)
		if err != nil {
			a.mutate(func() {
				reply.errText = err.Error()
				reply.done = true
				a.saveAll()
			})
			return
		}
		if res.Finish != "tool_calls" || len(res.ToolCalls) == 0 {
			break
		}
		if round >= maxToolRounds {
			a.mutate(func() { reply.errText = "工具调用轮次过多，已停止" })
			break
		}
		a.mutate(func() {
			reply.usedTools = true
			reply.step = "正在操作画板…"
		})
		// 只带工具调用时 content 必须是 null（空串会被严格网关判为 invalid tool_call）
		var assistantContent any
		if res.Content != "" {
			assistantContent = res.Content
		}
		messages = append(messages, OAMessage{Role: "assistant", Content: assistantContent, ToolCalls: res.ToolCalls})
		for _, tc := range res.ToolCalls {
			step := toolStep{Name: tc.Function.Name, Args: summarizeToolArgs(tc.Function.Arguments)}
			result := a.execTool(tc)
			step.Result, step.Failed = summarizeToolResult(result)
			a.mutate(func() { reply.tools = append(reply.tools, step) })
			messages = append(messages, OAMessage{Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: result})
		}
	}

	a.mutate(func() {
		reply.done = true
		if !reply.usedTools {
			a.applyGGBFences(reply) // 围栏兜底：仅当模型没用工具时
		}
		a.applyHTMLFence(reply)
		a.saveAll()
	})
}

// summarizeToolArgs 把工具参数压成一行可读摘要。
func summarizeToolArgs(argsJSON string) string {
	var args struct {
		Commands    []string `json:"commands"`
		Perspective string   `json:"perspective"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	parts := make([]string, 0, len(args.Commands))
	for _, c := range args.Commands {
		parts = append(parts, c)
	}
	s := strings.Join(parts, " ; ")
	if args.Perspective == "AT" {
		s = "[3D] " + s
	}
	if len([]rune(s)) > 90 {
		s = string([]rune(s)[:90]) + "…"
	}
	return s
}

// summarizeToolResult 把工具返回压成一行摘要。
func summarizeToolResult(result string) (string, bool) {
	var r struct {
		Applied int      `json:"applied"`
		Failed  []string `json:"failed"`
		Objects int      `json:"objects"`
		Count   int      `json:"objectCount"`
		Error   string   `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		return result, false
	}
	if r.Error != "" {
		return r.Error, true
	}
	out := "已应用 " + strconv.Itoa(r.Applied) + " 条"
	if len(r.Failed) > 0 {
		out += "，失败 " + strconv.Itoa(len(r.Failed)) + " 条"
	}
	if r.Objects > 0 {
		out += "，画板现有 " + strconv.Itoa(r.Objects) + " 个对象"
	}
	if r.Count > 0 {
		out = "画板共 " + strconv.Itoa(r.Count) + " 个对象"
	}
	return out, len(r.Failed) > 0
}

// execTool 执行模型请求的一个画板工具，返回给模型的 JSON 结果。
func (a *app) execTool(call OAToolCall) string {
	var args struct {
		Commands    []string `json:"commands"`
		Perspective string   `json:"perspective"`
	}
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	switch call.Function.Name {
	case "geogebra_run":
		r := a.boardCall(boardMsg{Op: "run", Perspective: args.Perspective, Commands: args.Commands}, 30*time.Second)
		if r.note != "" {
			return `{"error":"` + r.note + `"}`
		}
		// 画完后截一张图，供用户核对画得对不对
		if img := a.boardCall(boardMsg{Op: "image"}, 20*time.Second); img.image != "" {
			a.mutate(func() { a.boardImageURL = img.image })
		}
		out, _ := json.Marshal(map[string]any{
			"applied": r.applied,
			"failed":  r.errors,
			"objects": r.count,
			"names":   r.names,
		})
		return string(out)
	case "geogebra_read":
		r := a.boardCall(boardMsg{Op: "read"}, 15*time.Second)
		out, _ := json.Marshal(map[string]any{"objectCount": r.count, "names": r.names})
		return string(out)
	case "geogebra_export":
		r := a.boardCall(boardMsg{Op: "export"}, 30*time.Second)
		if r.b64 == "" {
			return `{"error":"画板没有返回内容"}`
		}
		data, err := base64.StdEncoding.DecodeString(r.b64)
		if err != nil {
			return `{"error":"` + err.Error() + `"}`
		}
		home, _ := os.UserHomeDir()
		path := filepath.Join(home, "Downloads", fmt.Sprintf("画板-%d.ggb", time.Now().Unix()))
		if werr := os.WriteFile(path, data, 0o644); werr != nil {
			return `{"error":"` + werr.Error() + `"}`
		}
		out, _ := json.Marshal(map[string]any{"saved": path})
		return string(out)
	default:
		return `{"error":"unknown tool ` + call.Function.Name + `"}`
	}
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
	a.sendBoardMsg(msg)
}

// boardBitmap 返回画板快照位图；解码需要主线程，缓存住避免重复解码。
func (a *app) boardBitmap() *ui.Bitmap {
	if a.boardImageURL == "" {
		return nil
	}
	if a.boardBitmapCache != nil {
		return a.boardBitmapCache
	}
	raw := a.boardImageURL
	if i := strings.Index(raw, ","); i >= 0 && strings.HasPrefix(raw, "data:") {
		raw = raw[i+1:]
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	bm, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	a.boardBitmapCache = bm
	return bm
}

// snapshotBoard 手动抓一张画板快照。
func (a *app) snapshotBoard() {
	r := a.boardCall(boardMsg{Op: "image"}, 20*time.Second)
	if r.image == "" {
		a.mutate(func() { a.boardNote = "截图失败" })
		return
	}
	a.mutate(func() { a.boardImageURL = r.image })
}

// boardOp 发一个非 run 的画板操作（export / read）。
func (a *app) boardOp(op string) {
	if a.ggbView == nil {
		return
	}
	a.boardSeq++
	a.sendBoardMsg(boardMsg{ID: "b" + strconv.Itoa(a.boardSeq), Op: op})
}

// openGGBFile 选一个 .ggb 文件读入画板。
func (a *app) openGGBFile() {
	if a.win == nil || a.boardBusy {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  a.win,
			Title:   "打开 GeoGebra 文件",
			Filters: []mygo.FileFilter{{Name: "GeoGebra", Extensions: []string{"ggb"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		data, readErr := os.ReadFile(paths[0])
		a.win.Update(func() {
			if readErr != nil {
				a.boardNote = "读取失败：" + readErr.Error()
				return
			}
			if a.ggbView == nil {
				return
			}
			a.boardSeq++
			a.boardBusy = true
			a.boardNote = ""
			a.sendBoardMsg(boardMsg{
				ID:     "b" + strconv.Itoa(a.boardSeq),
				Op:     "open",
				Base64: base64.StdEncoding.EncodeToString(data),
			})
		})
	}()
}

func (a *app) sendBoardMsg(msg boardMsg) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	ggbCmdsEvent.Broadcast(string(payload))
}

// deliverBoard 把画板回调结果送到等待中的 boardCall；返回是否有人接收。
func (a *app) deliverBoard(id string, r boardResult) bool {
	a.boardMu.Lock()
	ch := a.boardPending[id]
	a.boardMu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- r:
	default:
	}
	return true
}

// boardCall 同步执行一个画板操作并等待页面回报（ReAct 工具调用用）。
func (a *app) boardCall(msg boardMsg, timeout time.Duration) boardResult {
	if a.ggbView == nil { // headless：确定性占位结果
		switch msg.Op {
		case "export":
			return boardResult{b64: "ZGVtbw=="}
		case "read":
			return boardResult{count: 2, names: []string{"f", "A"}}
		default:
			return boardResult{applied: len(msg.Commands), count: len(msg.Commands)}
		}
	}
	a.boardSeq++
	msg.ID = "b" + strconv.Itoa(a.boardSeq)
	ch := make(chan boardResult, 1)
	a.boardMu.Lock()
	a.boardPending[msg.ID] = ch
	a.boardMu.Unlock()
	defer func() {
		a.boardMu.Lock()
		delete(a.boardPending, msg.ID)
		a.boardMu.Unlock()
	}()
	a.sendBoardMsg(msg)
	select {
	case r := <-ch:
		return r
	case <-time.After(timeout):
		return boardResult{note: "画板响应超时（" + timeout.String() + "）"}
	}
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
