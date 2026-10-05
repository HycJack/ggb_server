package aiModule

import (
	"bytes"
	"encoding/json"
	"ggb_server/internal/app/schema"
	"ggb_server/internal/consts"
	"io"
	"log"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func Test_reflect(t *testing.T) {
	client := &DouBaoChatCompletion{}

	typeC := reflect.TypeOf(client)
	valueC := reflect.ValueOf(client)

	structFieldType := typeC
	structFieldValue := valueC
	if reflect.TypeOf(client).Kind() == reflect.Pointer {
		structFieldType = structFieldType.Elem()
		structFieldValue = structFieldValue.Elem()
	}

	for i := 0; i < structFieldType.NumField(); i++ {
		fieldValue := structFieldValue.Field(i)
		fieldType := structFieldType.Field(i)
		log.Println("filedType.Type.Name(): ", fieldType.Type.Name())
		log.Println("filedType.Type.Kind(): ", fieldType.Type.Kind())
		log.Println("fieldType.Name: ", fieldType.Name)
		log.Println("fieldValue.CanSet(): ", fieldValue.CanSet())
		if fieldType.Type.Kind() == reflect.String && fieldValue.CanSet() {
			fieldValue.SetString("hello")
			log.Println("fieldValue.Interface(): ", fieldValue.Interface())
		}
		log.Println("----")
	}
}

func Test_NewClinet(t *testing.T) {
	client := NewChatCompletionClient[*DouBaoChatCompletion](map[string]string{
		"model": "model",
	}, nil, nil, nil)
	log.Println(client)
}

func Test_Chat(t *testing.T) {
	userContent := []schema.Content{
		schema.Content{
			Type: "text",
			Text: "描述这道题目, 同时使用中文回答 不要用英语",
		},
	}

	userContentBytes, err := json.Marshal(userContent)
	mapping := map[string]string{
		"model":                        string(consts.StepFuncChat1oTurbo),
		"message":                      string(userContentBytes),
		strings.ToLower("imgUrl"):      "http://106.52.162.78:8020/static/upload/1752302889160686111.jpg",
		strings.ToLower("processStep"): string(consts.Classify),
		strings.ToLower("contentType"): string(Classify),
	}
	client := NewChatCompletionClient[*StepFunChatCompletion](mapping, nil, nil, nil)

	res, err := client.ChatCompletion()

	if err != nil {
		log.Println("err: ", err)
		return
	}
	log.Println("res: ", res.Content)
}

func Test_ChatCompletion(t *testing.T) {
	client := NewChatCompletionClient[*ChatCompletionClient](map[string]string{
		"model":                        string(consts.TencentDeepSeek),
		"message":                      "你好 possible是什么意思",
		strings.ToLower("processStep"): string(consts.Classify),
	}, nil, nil, nil)

	res, err := client.ChatCompletion()
	if err != nil {
		log.Println("err: ", err)
	}
	log.Println("res: ", res.Content)
}

func Test_ChatCompletion2(t *testing.T) {
	chatCompletionReq := schema.DeepSeekRequest{
		Model: string(consts.DeepSeekChat),
		Messages: []schema.Message{
			schema.Message{
				Role:    "system",
				Content: "你是一名英语老师",
			},
			schema.Message{
				Role:    "user",
				Content: "possible的中文意思是什么",
			},
		},
		TopP:      1,
		MaxTokens: 1 << 13,
		Stream:    false, //非流式
	}
	payload, err := json.Marshal(chatCompletionReq)
	if err != nil {
		log.Println("err: ", err)
		return
	}
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, consts.DeepSeekChatCompletionUrl, bytes.NewBuffer(payload))

	if err != nil {
		log.Println("err: ", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+consts.DeepSeekApiKey)
	resp, err := client.Do(req)
	if err != nil {
		log.Println("err: ", err)
		return
	}
	defer resp.Body.Close()
	reader, _ := io.ReadAll(resp.Body)
	response := schema.DeepSeekResponse{}
	fullResponse := strings.Builder{}
	_ = json.Unmarshal(reader, &response)
	if len(response.Choices) > 0 {
		content := response.Choices[0].Message.Content
		fullResponse.WriteString(content)
	}
	log.Println("res: ", fullResponse.String())
}
