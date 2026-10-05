package aiModule

import (
	"ggb_server/internal/consts"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"log"
	"strings"
	"testing"
)

func Test_DouBao(t *testing.T) {

	client := NewChatCompletionClient[*DouBaoChatCompletion](
		map[string]string{
			"model":                         string(consts.DouBaoSeed1V6),
			"message":                       "possible有哪些用法",
			strings.ToLower("thinkingType"): string(model.ThinkingTypeEnabled),
		}, nil, nil, nil)
	client.douBaoClient = NewClient()

	completion, err := client.ChatCompletion()
	if err != nil {
		return
	}

	log.Println(completion)
}

func Test_DouBaoStream(t *testing.T) {

	client := NewChatCompletionClient[*DouBaoChatCompletion](
		map[string]string{
			"model":                         string(consts.DouBaoSeed1V6),
			"message":                       "介绍你自己",
			strings.ToLower("thinkingType"): string(model.ThinkingTypeEnabled),
		}, nil, nil, nil)
	client.douBaoClient = NewClient()

	completion, err := client.ChatCompletionStream()
	if err != nil {
		return
	}

	log.Println(completion)
}
