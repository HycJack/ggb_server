// 原生 OpenAI 格式的聊天客户端：POST {base}/chat/completions，SSE 流式。
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIClient 调用任意 OpenAI 兼容端点；可在任意 goroutine 使用。
type OpenAIClient struct {
	base string // 形如 https://api.deepseek.com/v1
	key  string
	http *http.Client
}

// NewOpenAIClient 归一化 base：去尾斜杠；不含 /v1 时补上（大多数兼容端点的约定）。
func NewOpenAIClient(base, key string) *OpenAIClient {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base != "" && !strings.Contains(base, "/v1") {
		base += "/v1"
	}
	return &OpenAIClient{
		base: base,
		key:  key,
		http: &http.Client{Timeout: 10 * time.Minute},
	}
}

// OAContentPart 是多模态消息的内容片段（纯文本或图片）。
type OAContentPart struct {
	Type     string      `json:"type"` // "text" | "image_url"
	Text     string      `json:"text,omitempty"`
	ImageURL *OAImageURL `json:"image_url,omitempty"`
}

type OAImageURL struct {
	URL string `json:"url"` // http(s) 或 data:image/png;base64,...
}

// OAToolCall 是模型请求的一次工具调用。
// 注意 function 必须是嵌套对象：{"id","type":"function","function":{"name","arguments"}}
// 摊平成同级字段会被严格网关判为 invalid tool_call。
type OAToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"` // 固定 "function"
	Function OAToolFunc `json:"function"`
}

// OAToolFunc 是工具调用的函数名与参数。
type OAToolFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// OAMessage 是一条对话消息；Content 为字符串（纯文本）或 []OAContentPart（带图）。
type OAMessage struct {
	Role       string       `json:"role"`
	Content    any          `json:"content"`                // 纯文本时是 string；只带工具调用时必须是 null
	ToolCalls  []OAToolCall `json:"tool_calls,omitempty"`   // role=assistant 请求的工具调用
	ToolCallID string       `json:"tool_call_id,omitempty"` // role=tool 的对应 ID
	Name       string       `json:"name,omitempty"`         // role=tool 的工具名
}

func textMessage(role, text string) OAMessage {
	return OAMessage{Role: role, Content: text}
}

// OAEvent 是流式回复的一个增量。
type OAEvent struct {
	Content   string // 正文增量
	Reasoning string // 思考增量（reasoning_content，DeepSeek 等提供）
}

// OAResult 是一次流式补全的汇总：正文增量已通过 onEvent 交付，
// 这里返回最终文本、模型请求的工具调用与结束原因。
type OAResult struct {
	Content   string
	ToolCalls []OAToolCall
	Finish    string // "stop" | "tool_calls" | ...
}

// BoardTools 是画板工具的 OpenAI function 定义。
var BoardTools = []map[string]any{
	{"type": "function", "function": map[string]any{
		"name": "geogebra_run",
		"description": "在 GeoGebra 画板上执行一批指令并立即生效。作图必须用这个工具。" +
			"每行一条指令，先创建对象再引用，例如 A=(1,2)、Segment(A,B)、Circle(A,2)、Polygon(A,B,C)、f(x)=sin(x)；" +
			"3D 对象用 Sphere/Cube/Pyramid 等，会自动切到 3D 视角。",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"commands": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
					"description": "GeoGebra 指令列表，每条一个字符串"},
				"perspective": map[string]any{"type": "string", "enum": []string{"AG", "AT"},
					"description": "可选。AG=代数+2D图形，AT=代数+3D图形；缺省保持当前视角"},
			},
			"required": []string{"commands"},
		},
	}},
	{"type": "function", "function": map[string]any{
		"name":        "geogebra_read",
		"description": "读取画板当前状态：对象数量与名字清单。检查画了什么、对象叫什么名字时调用。",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}},
	{"type": "function", "function": map[string]any{
		"name":        "geogebra_export",
		"description": "把当前画板导出为 .ggb 文件（保存到下载目录）。仅当用户明确要求保存/导出文件时调用。",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}},
}

// Chat 流式发起一次对话补全；每个增量经 onEvent 交付。
// tools 非 nil 时随请求下发 function 定义；返回汇总含工具调用。
func (c *OpenAIClient) Chat(model string, messages []OAMessage, onEvent func(OAEvent)) error {
	_, err := c.chat(model, messages, nil, onEvent)
	return err
}

func (c *OpenAIClient) chat(model string, messages []OAMessage, tools []map[string]any, onEvent func(OAEvent)) (OAResult, error) {
	reqBody := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}
	if tools != nil {
		reqBody["tools"] = tools
		reqBody["tool_choice"] = "auto"
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return OAResult{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return OAResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return OAResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return OAResult{}, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	result := OAResult{}
	type toolCallAcc struct {
		id, name string
		args     []byte
	}
	toolAcc := map[int]*toolCallAcc{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
					// OpenRouter 等网关用 reasoning 而不是 reasoning_content
					ReasoningAlt string `json:"reasoning"`
					ToolCalls    []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return result, fmt.Errorf("%s", chunk.Error.Message)
		}
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			d := choice.Delta
			reasoning := d.Reasoning
			if reasoning == "" {
				reasoning = d.ReasoningAlt
			}
			if d.Content != "" {
				result.Content += d.Content
			}
			for _, tc := range d.ToolCalls {
				acc := toolAcc[tc.Index]
				if acc == nil {
					acc = &toolCallAcc{}
					toolAcc[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				acc.args = append(acc.args, tc.Function.Arguments...)
			}
			if choice.FinishReason != "" {
				result.Finish = choice.FinishReason
			}
			if d.Content != "" || reasoning != "" {
				onEvent(OAEvent{Content: d.Content, Reasoning: reasoning})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return result, err
	}
	for i := 0; i < len(toolAcc); i++ {
		acc := toolAcc[i]
		if acc == nil {
			continue
		}
		id := acc.id
		if id == "" { // 有些网关不在流里给 id，补一个占位
			id = fmt.Sprintf("call_%d", i)
		}
		args := strings.TrimSpace(string(acc.args))
		if args == "" {
			args = "{}"
		}
		result.ToolCalls = append(result.ToolCalls, OAToolCall{
			ID:       id,
			Type:     "function",
			Function: OAToolFunc{Name: acc.name, Arguments: args},
		})
	}
	return result, nil
}

// Models 拉取 /models 列表，供设置页校验连通性（失败不阻塞保存）。
func (c *OpenAIClient) Models() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, c.base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// extractGGBBlocks 从 markdown 文本中抽取 ```ggb / ```geogebra 围栏里的指令。
func extractGGBBlocks(text string) []string {
	var out []string
	blocks := extractFenced(text, "ggb")
	blocks = append(blocks, extractFenced(text, "geogebra")...)
	for _, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
				continue
			}
			out = append(out, line)
		}
	}
	return out
}
