package aiModule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ggb_server/internal/consts"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
	"io"
	"net/http"
	"strings"
	"time"
)

type DouBaoChatCompletion struct {
	Ctx          context.Context
	Model        consts.DouBaoModel
	ProcessStep  consts.ProcessStep
	Message      string
	Flusher      http.Flusher
	StreamWriter io.Writer
	ContentType  Type
	ImgUrl       string

	douBaoClient *arkruntime.Client
	ThinkingType model.ThinkingType
	UserInfo     *UserInfo
}

func NewClient() *arkruntime.Client {
	return arkruntime.NewClientWithApiKey(consts.DouBaoApiKey, arkruntime.WithTimeout(30*time.Minute))
}

func (d DouBaoChatCompletion) ChatCompletion() (Content, error) {
	if d.douBaoClient == nil {
		return Content{}, errors.New("douBao client is nil, please init client first")
	}
	req := model.CreateChatCompletionRequest{
		Model: string(d.Model),
		Messages: []*model.ChatCompletionMessage{
			{
				Role: model.ChatMessageRoleSystem,
				Content: &model.ChatCompletionMessageContent{
					StringValue: volcengine.String(consts.ProcessStepMapping[d.ProcessStep]),
				},
			},
			{
				Role: model.ChatMessageRoleUser,
				Content: &model.ChatCompletionMessageContent{
					StringValue: volcengine.String(d.Message),
				},
			},
		},
		Thinking: &model.Thinking{
			Type: d.ThinkingType,
		},
	}
	if d.ImgUrl != "" {
		req.Messages[0] = &model.ChatCompletionMessage{
			Role: model.ChatMessageRoleSystem,
			Content: &model.ChatCompletionMessageContent{
				ListValue: []*model.ChatCompletionMessageContentPart{
					{
						Type: "image_url",
						ImageURL: &model.ChatMessageImageURL{
							URL: d.ImgUrl,
						},
					},
					{
						Type: "text",
						Text: consts.ProcessStepMapping[d.ProcessStep],
					},
				},
			},
		}
	}
	ctx := d.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := d.douBaoClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return Content{}, err
	}
	fullResponse := strings.Builder{}
	if resp.Choices[0].Message.ReasoningContent != nil {
		content := *resp.Choices[0].Message.Content.StringValue
		formatContent := Content{
			Type:    OutputContent,
			Step:    d.ProcessStep,
			Content: content,
		}
		jsonBody, _ := json.Marshal(formatContent)
		fullResponse.WriteString(content)
		fmt.Println(content)
		writeSSEEvent(d.StreamWriter, d.Flusher, string(jsonBody))
	}
	if fullResponse.Len() > 0 {
		err = insertAiMessage(d.UserInfo, fullResponse.String(), string(d.Model), false, d.ProcessStep)
	}
	return Content{
		Type:    OutputContent,
		Content: fullResponse.String(),
		Step:    d.ProcessStep,
	}, nil
}

func (d DouBaoChatCompletion) ChatCompletionStream() (Content, error) {
	ctx := d.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req := model.CreateChatCompletionRequest{
		Model: string(d.Model),
		Messages: []*model.ChatCompletionMessage{
			{
				Role: model.ChatMessageRoleSystem,
				Content: &model.ChatCompletionMessageContent{
					StringValue: volcengine.String(consts.ProcessStepMapping[d.ProcessStep]),
				},
			},
			{
				Role: model.ChatMessageRoleUser,
				Content: &model.ChatCompletionMessageContent{
					StringValue: volcengine.String(d.Message),
				},
			},
		},
	}
	if d.ImgUrl != "" {
		req.Messages[0] = &model.ChatCompletionMessage{
			Role: model.ChatMessageRoleSystem,
			Content: &model.ChatCompletionMessageContent{
				ListValue: []*model.ChatCompletionMessageContentPart{
					{
						Type: "image_url",
						ImageURL: &model.ChatMessageImageURL{
							URL: d.ImgUrl,
						},
					},
					{
						Type: "text",
						Text: consts.ProcessStepMapping[d.ProcessStep],
					},
				},
			},
		}
	}

	stream, err := d.douBaoClient.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return Content{}, err
	}
	defer stream.Close()

	fullResponse := strings.Builder{}
	reasoningResponse := strings.Builder{}
	recvTimeout := 10 * time.Second
	recvTimer := time.NewTimer(recvTimeout)
	defer recvTimer.Stop()

	for {
		select {
		case <-recvTimer.C:
			// 规定时间没有收到新数据
			break
		default:
		}

		recv, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Content{}, err
		}

		if !recvTimer.Stop() {
			<-recvTimer.C
		}
		recvTimer.Reset(recvTimeout)

		if len(recv.Choices) > 0 && recv.Choices[0].Delta.ReasoningContent != nil {
			reasoningContent := *recv.Choices[0].Delta.ReasoningContent
			formatContent := Content{
				Type:    Reasoning,
				Step:    d.ProcessStep,
				Content: reasoningContent,
			}
			fmt.Print(reasoningContent)
			reasoningResponse.WriteString(reasoningContent)
			jsonBody, _ := json.Marshal(formatContent)
			writeSSEEvent(d.StreamWriter, d.Flusher, string(jsonBody))
		}

		if len(recv.Choices) > 0 && recv.Choices[0].Delta.Content != "" {
			content := recv.Choices[0].Delta.Content
			formatContent := Content{
				Type:    OutputContent,
				Step:    d.ProcessStep,
				Content: content,
			}
			jsonBody, _ := json.Marshal(formatContent)
			writeSSEEvent(d.StreamWriter, d.Flusher, string(jsonBody))
			fullResponse.WriteString(content)
		}
	}

	if reasoningResponse.Len() > 0 {
		err = insertAiMessage(d.UserInfo, reasoningResponse.String(), string(d.Model), true, d.ProcessStep)
	}
	if fullResponse.Len() > 0 {
		err = insertAiMessage(d.UserInfo, fullResponse.String(), string(d.Model), false, d.ProcessStep)
	}

	return Content{
		Type:    OutputContent,
		Content: fullResponse.String(),
		Step:    d.ProcessStep,
	}, nil
}
