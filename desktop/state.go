package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HycJack/MujicaUI/chat"
)

// settings 是落盘的认证与会话配置。
type settings struct {
	ServerURL string `json:"serverURL"`
	Token     string `json:"token"`
	Username  string `json:"username"`
	UserID    string `json:"userId"`
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ggb-desktop", "settings.json"), nil
}

func loadSettings() settings {
	var s settings
	s.ServerURL = "http://localhost:8080"
	if p, err := settingsPath(); err == nil {
		if data, err := os.ReadFile(p); err == nil {
			json.Unmarshal(data, &s)
			if s.ServerURL == "" {
				s.ServerURL = "http://localhost:8080"
			}
		}
	}
	return s
}

func saveSettings(s settings) {
	p, err := settingsPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(p, data, 0o600)
}

// uiMsg 是视图层的消息：一条用户提问或一条助手回复。
// 助手回复的四个内容区随 SSE 事件逐步填充。
type uiMsg struct {
	id   string
	role chat.MessageRole

	text     string // 用户气泡正文 / 助手的元素提取文本（markdown）
	thinking string // reasoning 事件累积的思考内容
	ggb      string // ggb_commands 事件
	html     string // html_code 事件（GeoGebra 页面）
	errText  string // 失败信息

	step    string // 当前进行到的步骤名
	done    bool
	open    bool      // 思考块默认展开
	started time.Time // 回复开始的时间（ThinkingBlock 需要）
}

// stepLabel 把服务端 ProcessStep 值映射成中文标签。
func stepLabel(step string) string {
	s := strings.ToLower(step)
	switch {
	case strings.Contains(s, "generatehtml"):
		return "生成 GeoGebra 页面"
	case strings.Contains(s, "generateggb"):
		return "生成 GGB 指令"
	case strings.Contains(s, "extract"):
		return "提取数学元素"
	case strings.Contains(s, "classify"):
		return "识别题目类型"
	default:
		return "处理中"
	}
}

// toUiMsgs 把服务端历史转成视图消息：每条用户消息跟一条助手回复。
func toUiMsgs(hist []histMessage) []*uiMsg {
	var out []*uiMsg
	n := 0
	next := func() string { n++; return "h" + strconv.Itoa(n) }
	for _, m := range hist {
		if m.Identity != 0 {
			continue
		}
		out = append(out, &uiMsg{id: next(), role: chat.MessageUser, text: m.Message, done: true})
		reply := &uiMsg{id: next(), role: chat.MessageAssistant, done: true}
		for _, ai := range m.AiMessages {
			switch {
			case ai.IsReason:
				if reply.thinking != "" {
					reply.thinking += "\n\n"
				}
				reply.thinking += ai.Message
			case ai.Stage == 3:
				reply.html = ai.Message
			default:
				if reply.text != "" {
					reply.text += "\n\n"
				}
				reply.text += ai.Message
			}
		}
		if reply.text != "" || reply.thinking != "" || reply.html != "" {
			out = append(out, reply)
		}
	}
	return out
}

func truncTitle(s string) string {
	r := []rune(s)
	if len(r) > 24 {
		return string(r[:24])
	}
	return s
}

var _ = time.Now // 保持 time 依赖（settings 时间字段）
