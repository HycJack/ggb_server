// Package main 的 API 客户端：ggb_server 的登录、会话与 SSE 聊天接口。
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

// Client 调用 ggb_server 的 HTTP API；所有方法可在任意 goroutine 调用。
type Client struct {
	base string
	http *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: 5 * time.Minute},
	}
}

// ---- 协议类型（与服务端的 JSON 标签对应） ----

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string        `json:"access_token"`
	User        loginUserInfo `json:"user"`
	Error       string        `json:"error"`
}

type loginUserInfo struct {
	ID               uint   `json:"id"`
	UserId           string `json:"userId"`
	Username         string `json:"username"`
	FreeMessageCount int    `json:"freeMessageCount"`
}

type conversation struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type conversationsResponse struct {
	Total         int            `json:"total"`
	Conversations []conversation `json:"conversations"`
}

type histAiMessage struct {
	Message   string `json:"message"`
	ModelName string `json:"modelName"`
	Stage     int    `json:"stage"`
	IsReason  bool   `json:"isReason"`
}

type histResource struct {
	URL string `json:"url"`
}

type histMessage struct {
	ID         uint            `json:"id"`
	Message    string          `json:"message"`
	UserID     string          `json:"userId"`
	Identity   int             `json:"identity"`
	CreatedAt  time.Time       `json:"createdAt"`
	AiMessages []histAiMessage `json:"aiMessage"`
	Resources  []histResource  `json:"resource"`
}

type historyResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    struct {
		Message []histMessage `json:"message"`
	} `json:"data"`
}

// SSEEvent 是服务端 SSE data 行里 {"data":{...}} 的内层对象。
type SSEEvent struct {
	Type    string `json:"type"` // reasoning | element | ggb_commands | html_code | error | failed_text
	Step    string `json:"step"` // classify | 2DExtract | 2DGenerateGGB | 2DGenerateHTML ...
	Content string `json:"content"`
}

// ---- 接口 ----

func (c *Client) do(method, path, token string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// Login 登录并返回 token 与用户信息。
func (c *Client) Login(username, password string) (string, loginUserInfo, error) {
	var out loginResponse
	if err := c.do(http.MethodPost, "/api/login", "", loginRequest{username, password}, &out); err != nil {
		return "", loginUserInfo{}, err
	}
	if out.AccessToken == "" {
		if out.Error != "" {
			return "", loginUserInfo{}, fmt.Errorf("%s", out.Error)
		}
		return "", loginUserInfo{}, fmt.Errorf("登录失败：响应中没有 token")
	}
	return out.AccessToken, out.User, nil
}

// Conversations 拉取会话列表。
func (c *Client) Conversations(token string) ([]conversation, error) {
	var out conversationsResponse
	if err := c.do(http.MethodGet, "/api/conversations?page=1&pageSize=100", token, nil, &out); err != nil {
		return nil, err
	}
	return out.Conversations, nil
}

// CreateConversation 新建会话，返回新会话 ID。
func (c *Client) CreateConversation(token, title string) (uint, error) {
	var out struct {
		ID uint `json:"id"`
	}
	if err := c.do(http.MethodPost, "/api/conversations", token, map[string]string{"title": title}, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// DeleteConversation 删除会话。
func (c *Client) DeleteConversation(token string, id uint) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/conversations/%d", id), token, nil, nil)
}

// History 拉取一个会话的完整消息历史。
func (c *Client) History(token string, id uint) ([]histMessage, error) {
	var out historyResponse
	if err := c.do(http.MethodGet, fmt.Sprintf("/api/conversations/%d", id), token, nil, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		if out.Error != "" {
			return nil, fmt.Errorf("%s", out.Error)
		}
		return nil, fmt.Errorf("获取历史失败")
	}
	return out.Data.Message, nil
}

// Chat 发起一条消息并流式读取 SSE 事件；每个事件通过 onEvent 回调交付。
func (c *Client) Chat(token string, sessionID uint, message string, onEvent func(SSEEvent)) error {
	body, err := json.Marshal(map[string]any{
		"message":   message,
		"sessionId": sessionID,
		"parentId":  0,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.base+"/api/v2/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
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
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // html_code 事件是一条超长行
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var wrapper struct {
			Data SSEEvent `json:"data"`
		}
		if err := json.Unmarshal([]byte(payload), &wrapper); err != nil {
			continue // 无法解析的心跳/杂项行直接跳过
		}
		onEvent(wrapper.Data)
	}
	return sc.Err()
}
