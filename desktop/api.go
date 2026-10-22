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

// OAMessage 是一条对话消息；Content 为字符串（纯文本）或 []OAContentPart（带图）。
type OAMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func textMessage(role, text string) OAMessage {
	return OAMessage{Role: role, Content: text}
}

// OAEvent 是流式回复的一个增量。
type OAEvent struct {
	Content   string // 正文增量
	Reasoning string // 思考增量（reasoning_content，DeepSeek 等提供）
}

// Chat 流式发起一次对话补全；每个增量经 onEvent 交付。
func (c *OpenAIClient) Chat(model string, messages []OAMessage, onEvent func(OAEvent)) error {
	body, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   true,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

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
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return fmt.Errorf("%s", chunk.Error.Message)
		}
		if len(chunk.Choices) > 0 {
			d := chunk.Choices[0].Delta
			if d.Content != "" || d.Reasoning != "" {
				onEvent(OAEvent{Content: d.Content, Reasoning: d.Reasoning})
			}
		}
	}
	return sc.Err()
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
