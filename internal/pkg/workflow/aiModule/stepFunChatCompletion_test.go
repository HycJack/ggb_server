package aiModule

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"ggb_server/internal/app/schema"
	"ggb_server/internal/config"
	"ggb_server/internal/consts"
	"ggb_server/internal/utils"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_RootPath(t *testing.T) {
	rootPath, _ := utils.FindRootPath()
	fmt.Println(rootPath)
}

func Test_StepFuncWithBase64Image(t *testing.T) {
	if consts.StepFunApiKey == "" {
		t.Skip("no api key configured, skipping integration test")
	}
	rootPath, _ := utils.FindRootPath()
	filePath := filepath.Join(rootPath, "static/upload/1752091241079782000.png")
	file, _ := os.Open(filePath)
	defer file.Close()

	fileContent, _ := io.ReadAll(file)

	base64Image := base64.StdEncoding.EncodeToString(fileContent)

	contents := schema.UserMessageContent{
		schema.TextType{
			Type: "text",
			Text: "描述这个题目 如果有选项则包含选项一起放到'题目'中进行描述",
		},
	}

	contents = append(contents, schema.ImageType{
		Type: "image_url",
		ImageUrl: struct {
			Url    string `json:"url"`
			Detail string `json:"detail"`
		}(struct {
			Url    string
			Detail string
		}{
			Url:    fmt.Sprintf("data:image/png;base64,%s", base64Image),
			Detail: "high",
		}),
	})

	type StepFunChatCompletionRequest struct {
		Model    string             `json:"model"`
		Messages schema.ChatMessage `json:"messages"`
		Stream   bool               `json:"stream"`
	}

	req := StepFunChatCompletionRequest{
		Model: "step-1v-8k",
		Messages: schema.ChatMessage{
			schema.SystemMessage{
				Role:    "system",
				Content: consts.ClassificationSystemPrompt,
			},
			schema.UserMessage{
				Role:    "user",
				Content: contents,
			},
		},
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return
	}

	log.Printf("marshal res: %s", string(reqBytes))

	client := &http.Client{}

	chatCompletionReq, err := http.NewRequest(http.MethodPost, consts.StepFunChatCompletionUrl, bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Println("NewRequest err:", err)
		return
	}

	chatCompletionReq.Header.Set("Content-Type", "application/json")
	chatCompletionReq.Header.Set("Authorization", "Bearer "+consts.StepFunApiKey)
	resp, err := client.Do(chatCompletionReq)
	if err != nil {
		log.Println("Do err:", err)
		return
	}
	defer resp.Body.Close()

	reader, _ := io.ReadAll(resp.Body)

	chatCompletionResp := schema.StepFunChatCompletionResponse{}
	err = json.Unmarshal(reader, &chatCompletionResp)
	log.Printf("resp: %+v\n", chatCompletionResp)

	log.Println("content: ", chatCompletionResp.Choices[0].Message.Content)

}

func Test_StepFunWithImageUrl(t *testing.T) {
	if consts.StepFunApiKey == "" {
		t.Skip("no api key configured, skipping integration test")
	}
	contents := schema.UserMessageContent{
		schema.TextType{
			Type: "text",
			Text: "描述这个题目 如果有选项则包含选项一起放到'题目'中进行描述",
		},
	}

	contents = append(contents, schema.ImageType{
		Type: "image_url",
		ImageUrl: struct {
			Url    string `json:"url"`
			Detail string `json:"detail"`
		}(struct {
			Url    string
			Detail string
		}{
			Url:    "http://106.52.162.78:8020/static/upload/1752091241079782000.png",
			Detail: "high",
		}),
	})

	type StepFunChatCompletionRequest struct {
		Model    string             `json:"model"`
		Messages schema.ChatMessage `json:"messages"`
		Stream   bool               `json:"stream"`
	}

	req := StepFunChatCompletionRequest{
		Model: "step-1v-8k",
		Messages: schema.ChatMessage{
			schema.SystemMessage{
				Role:    "system",
				Content: consts.ClassificationSystemPrompt,
			},
			schema.UserMessage{
				Role:    "user",
				Content: contents,
			},
		},
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return
	}

	log.Printf("marshal res: %s", string(reqBytes))

	client := &http.Client{}

	chatCompletionReq, err := http.NewRequest(http.MethodPost, consts.StepFunChatCompletionUrl, bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Println("NewRequest err:", err)
		return
	}

	chatCompletionReq.Header.Set("Content-Type", "application/json")
	chatCompletionReq.Header.Set("Authorization", "Bearer "+consts.StepFunApiKey)
	resp, err := client.Do(chatCompletionReq)
	if err != nil {
		log.Println("Do err:", err)
		return
	}
	defer resp.Body.Close()

	reader, _ := io.ReadAll(resp.Body)

	chatCompletionResp := schema.StepFunChatCompletionResponse{}
	err = json.Unmarshal(reader, &chatCompletionResp)
	log.Printf("resp: %+v\n", chatCompletionResp)

	log.Println("content: ", chatCompletionResp.Choices[0].Message.Content)
}

func TestImgUrl(t *testing.T) {
	url := "http://localhost:8081/static/upload/1752324138383213000.jpg"
	path := url[strings.Index(url, config.Cfg.Static.Path):]
	rootPath, _ := utils.FindRootPath()
	imgPath := filepath.Join(rootPath, path)
	log.Println(imgPath)
}
