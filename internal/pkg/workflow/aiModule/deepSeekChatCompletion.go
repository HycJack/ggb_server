package aiModule

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"ggb_server/internal/app/schema"
	"ggb_server/internal/consts"
	"golang.org/x/net/context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

type ChatCompletionClient struct {
	Ctx          context.Context
	Model        consts.DeepSeekModel
	ProcessStep  consts.ProcessStep
	Message      string
	Flusher      http.Flusher
	StreamWriter io.Writer
	ContentType  Type
	UserInfo     *UserInfo
}

func (g ChatCompletionClient) ChatCompletion() (Content, error) {
	chatCompletionReq := schema.DeepSeekRequest{
		Model: string(g.Model),
		Messages: []schema.Message{
			schema.Message{
				Role:    "system",
				Content: consts.ProcessStepMapping[g.ProcessStep],
			},
			schema.Message{
				Role:    "user",
				Content: g.Message,
			},
		},
		TopP:      1,
		MaxTokens: 1 << 13,
		Stream:    false, //非流式
	}
	payload, err := json.Marshal(chatCompletionReq)
	if err != nil {
		log.Println("json marshal error:", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    g.ProcessStep,
		}, err
	}
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, consts.DeepSeekChatCompletionUrl, bytes.NewBuffer(payload))
	if err != nil {
		log.Println("new request error:", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    g.ProcessStep,
		}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+consts.DeepSeekApiKey)
	resp, err := client.Do(req)
	if err != nil {
		log.Print("send request error:", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    g.ProcessStep,
		}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		log.Println("API returned error: ", string(body))
		return Content{
			Type:    Error,
			Content: string(body),
			Step:    g.ProcessStep,
		}, errors.New("API returned error: " + resp.Status)
	}

	reader, _ := io.ReadAll(resp.Body)
	response := schema.DeepSeekResponse{}
	fullResponse := strings.Builder{}
	_ = json.Unmarshal(reader, &response)
	if len(response.Choices) > 0 {
		content := response.Choices[0].Message.Content
		if (g.ProcessStep == consts.TwoDGenerateHTML) || (g.ProcessStep == consts.ThreeDGenerateHTML) || (g.ProcessStep == consts.FunctionGenerateHTML) || (g.ProcessStep == consts.KnowledgeGenerateHTML) {
			content = filterHTML(content)
		}
		formatContent := Content{
			Type:    OutputContent,
			Step:    g.ProcessStep,
			Content: content,
		}
		if (g.ProcessStep == consts.TwoDGenerateHTML) || g.ProcessStep == consts.ThreeDGenerateHTML || g.ProcessStep == consts.FunctionGenerateHTML {
			formatContent.Type = GGBCommands
		}
		if (g.ProcessStep == consts.TwoDGenerateHTML) || (g.ProcessStep == consts.ThreeDGenerateHTML) || (g.ProcessStep == consts.FunctionGenerateHTML) || (g.ProcessStep == consts.KnowledgeGenerateHTML) {
			formatContent.Type = HTMLCode
		}
		jsonBody, _ := json.Marshal(formatContent)
		fullResponse.WriteString(content)
		writeSSEEvent(g.StreamWriter, g.Flusher, string(jsonBody)) //也以流式形式返回前端
	}

	if fullResponse.Len() > 0 {
		err = insertAiMessage(g.UserInfo, fullResponse.String(), string(g.Model), false, g.ProcessStep)
	}

	return Content{
		Type:    OutputContent,
		Content: fullResponse.String(),
		Step:    g.ProcessStep,
	}, nil
}

func (g ChatCompletionClient) ChatCompletionStream() (Content, error) {
	chatCompletionReq := schema.DeepSeekRequest{
		Model: string(g.Model),
		Messages: []schema.Message{
			{
				Role:    "system",
				Content: consts.ProcessStepMapping[g.ProcessStep],
			},
			{
				Role:    "user",
				Content: g.Message,
			},
		},
		TopP:      1,
		MaxTokens: 1024,
		Stream:    true, // 流式
	}

	reqBody, err := json.Marshal(chatCompletionReq)
	if err != nil {
		log.Println("Error marshaling request: ", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    g.ProcessStep,
		}, err
	}

	req, err := http.NewRequest(http.MethodPost, consts.DeepSeekChatCompletionUrl, bytes.NewBuffer(reqBody))
	if err != nil {
		log.Println("Error creating request: ", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    g.ProcessStep,
		}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+consts.DeepSeekApiKey)
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error sending request: ", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    g.ProcessStep,
		}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Println("API returned non-OK : ", string(bodyBytes))
		return Content{
			Type:    Error,
			Content: string(bodyBytes),
			Step:    g.ProcessStep,
		}, errors.New("API returned non-OK : " + string(bodyBytes))
	}

	fullResponse := strings.Builder{}
	reasoningResponse := strings.Builder{}
	reader := bufio.NewReader(resp.Body)

	recvTimeout := 10 * time.Second
	recvTimer := time.NewTimer(recvTimeout)
	defer recvTimer.Stop()

	for {
		select {
		case <-recvTimer.C:
			// 规定时间没有收到新数据，返回错误
			break
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Println("Error reading stream: ", err)
		}

		if !recvTimer.Stop() {
			<-recvTimer.C
		}
		recvTimer.Reset(recvTimeout)

		if strings.HasPrefix(line, "data: ") {
			jsonStr := strings.TrimPrefix(line, "data: ")
			jsonStr = strings.TrimSpace(jsonStr)

			if jsonStr == "[DONE]" {
				break
			}

			var chunk schema.DeepSeekStreamResponse
			if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
				log.Printf("unmarshal chunk error: %v, content: %s", err, jsonStr)
				continue
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.ReasoningContent != "" {
				reasoningContent := chunk.Choices[0].Delta.ReasoningContent
				formatContent := Content{
					Type:    Reasoning,
					Step:    g.ProcessStep,
					Content: reasoningContent,
				}
				fmt.Print(reasoningContent)
				reasoningResponse.WriteString(reasoningContent)
				jsonBody, _ := json.Marshal(formatContent)
				writeSSEEvent(g.StreamWriter, g.Flusher, string(jsonBody))
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				content := chunk.Choices[0].Delta.Content
				formatContent := Content{
					Type:    OutputContent,
					Step:    g.ProcessStep,
					Content: content,
				}
				jsonBody, _ := json.Marshal(formatContent)
				writeSSEEvent(g.StreamWriter, g.Flusher, string(jsonBody))
				fullResponse.WriteString(content)
			}
		}
	}

	if reasoningResponse.Len() > 0 {
		err = insertAiMessage(g.UserInfo, reasoningResponse.String(), string(g.Model), true, g.ProcessStep)
	}
	if fullResponse.Len() > 0 {
		err = insertAiMessage(g.UserInfo, fullResponse.String(), string(g.Model), false, g.ProcessStep)
	}

	return Content{
		Type:    OutputContent,
		Content: fullResponse.String(),
		Step:    g.ProcessStep,
	}, nil
}
