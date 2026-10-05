package aiModule

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"ggb_server/internal/app/schema"
	"ggb_server/internal/consts"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

type StepFunChatCompletion struct {
	Model        consts.StepFunModel
	ProcessStep  consts.ProcessStep
	Message      string
	Flusher      http.Flusher
	StreamWriter io.Writer
	ImgUrl       string

	UserInfo *UserInfo
}

func (s StepFunChatCompletion) ChatCompletion() (Content, error) {
	content := schema.UserMessageContent{
		schema.TextType{
			Type: "text",
			Text: s.Message,
		},
	}
	if s.ImgUrl != "" {
		content = append(content, schema.ImageType{
			Type: "image_url",
			ImageUrl: struct {
				Url    string `json:"url"`
				Detail string `json:"detail"`
			}(struct {
				Url    string
				Detail string
			}{Url: s.ImgUrl, Detail: "high"}),
		})
	}

	chatCompletionReq := schema.StepFunChatCompletionRequest{
		Model: string(s.Model),
		Messages: schema.ChatMessage{
			schema.SystemMessage{
				Role:    "system",
				Content: consts.ProcessStepMapping[s.ProcessStep],
			},
			schema.UserMessage{
				Role:    "user",
				Content: content,
			},
		},
		Stream: false,
	}

	payload, err := json.Marshal(chatCompletionReq)
	if err != nil {
		log.Println("json marshal error:", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    s.ProcessStep,
		}, err
	}
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, consts.StepFunChatCompletionUrl, bytes.NewBuffer(payload))
	if err != nil {
		log.Println("new request error:", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    s.ProcessStep,
		}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+consts.StepFunApiKey)
	resp, err := client.Do(req)
	if err != nil {
		log.Print("send request error:", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    s.ProcessStep,
		}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		log.Println("API returned error: ", string(body))
		return Content{
			Type:    Error,
			Content: string(body),
			Step:    s.ProcessStep,
		}, errors.New("API returned error: " + resp.Status)
	}

	reader, _ := io.ReadAll(resp.Body)
	response := schema.StepFunChatCompletionResponse{}
	fullResponse := strings.Builder{}
	_ = json.Unmarshal(reader, &response)
	if len(response.Choices) > 0 {
		outputContent := response.Choices[0].Message.Content
		formatContent := Content{
			Type:    Reasoning,
			Step:    s.ProcessStep,
			Content: outputContent,
		}
		jsonBody, _ := json.Marshal(formatContent)
		fullResponse.WriteString(outputContent)
		fmt.Print(outputContent)
		writeSSEEvent(s.StreamWriter, s.Flusher, string(jsonBody)) //也以流式形式返回前端
	}
	if fullResponse.Len() > 0 {
		err = insertAiMessage(s.UserInfo, fullResponse.String(), string(s.Model), false, s.ProcessStep)
	}
	return Content{
		Type:    OutputContent,
		Content: fullResponse.String(),
		Step:    s.ProcessStep,
	}, nil
}

func (s StepFunChatCompletion) ChatCompletionStream() (Content, error) {
	content := schema.UserMessageContent{
		schema.TextType{
			Type: "text",
			Text: s.Message,
		},
	}
	if s.ImgUrl != "" {
		content = append(content, schema.ImageType{
			Type: "image_url",
			ImageUrl: struct {
				Url    string `json:"url"`
				Detail string `json:"detail"`
			}(struct {
				Url    string
				Detail string
			}{Url: s.ImgUrl, Detail: "high"}),
		})
	}

	chatCompletionReq := schema.StepFunChatCompletionRequest{
		Model: string(s.Model),
		Messages: schema.ChatMessage{
			schema.SystemMessage{
				Role:    "system",
				Content: consts.ProcessStepMapping[s.ProcessStep],
			},
			schema.UserMessage{
				Role:    "user",
				Content: content,
			},
		},
		Stream: true,
	}

	reqBody, err := json.Marshal(chatCompletionReq)
	if err != nil {
		log.Println("Error marshalling user content error: ", err)
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    s.ProcessStep,
		}, err
	}

	req, err := http.NewRequest(http.MethodPost, consts.StepFunChatCompletionUrl, bytes.NewBuffer(reqBody))
	if err != nil {
		return Content{
			Type:    Error,
			Content: err.Error(),
			Step:    s.ProcessStep,
		}, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "text/event-stream")
	req.Header.Add("Authorization", "Bearer "+consts.StepFunApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Content{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Content{
			Type:    Error,
			Content: "",
			Step:    s.ProcessStep,
		}, errors.New(resp.Status)
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

		if strings.HasPrefix(line, "data:") {
			jsonStr := strings.Trim(line, "data: ")
			jsonStr = strings.TrimSpace(jsonStr)

			if jsonStr == "[DONE]" {
				break
			}

			var chunk schema.StepFunChatCompletionStreamResponse
			if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Reasoning != "" {
				reasoningContent := chunk.Choices[0].Delta.Reasoning
				formatContent := Content{
					Type:    Reasoning,
					Step:    s.ProcessStep,
					Content: reasoningContent,
				}
				reasoningResponse.WriteString(reasoningContent)
				fmt.Print(reasoningContent)
				jsonBody, _ := json.Marshal(formatContent)
				writeSSEEvent(s.StreamWriter, s.Flusher, string(jsonBody))
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				output := chunk.Choices[0].Delta.Content
				formatContent := Content{
					Type:    OutputContent,
					Step:    s.ProcessStep,
					Content: output,
				}
				jsonBody, _ := json.Marshal(formatContent)
				writeSSEEvent(s.StreamWriter, s.Flusher, string(jsonBody))
				fullResponse.WriteString(output)
			}
		}
	}

	if reasoningResponse.Len() > 0 {
		err = insertAiMessage(s.UserInfo, reasoningResponse.String(), string(s.Model), true, s.ProcessStep)
	}
	if fullResponse.Len() > 0 {
		err = insertAiMessage(s.UserInfo, fullResponse.String(), string(s.Model), false, s.ProcessStep)
	}

	return Content{
		Type:    OutputContent,
		Content: fullResponse.String(),
		Step:    s.ProcessStep,
	}, nil
}
