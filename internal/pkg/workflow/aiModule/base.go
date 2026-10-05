package aiModule

import (
	"fmt"
	"ggb_server/internal/app/model"
	"ggb_server/internal/consts"
	"ggb_server/internal/repository"
	"io"
	"net/http"
	"reflect"
	"strings"
)

const (
	Classify = 0
	Extract  = 1
	GenGGB   = 2
	GenHTML  = 3
)

var stageMapping = map[string]int{
	"classify":     Classify,
	"extract":      Extract,
	"generateGGB":  GenGGB,
	"generateHTML": GenHTML,
}

func NewChatCompletionClient[T ChatCompletionInterface](mapping map[string]string, flusher http.Flusher, w io.Writer, userInfo *UserInfo) T {
	var zero T
	elemType := reflect.TypeOf(zero).Elem()
	clientValue := reflect.New(elemType)

	if flusher != nil {
		if fField := clientValue.Elem().FieldByName("Flusher"); fField.IsValid() && fField.CanSet() {
			fField.Set(reflect.ValueOf(flusher))
		}
	}
	if w != nil {
		if swField := clientValue.Elem().FieldByName("StreamWriter"); swField.IsValid() && swField.CanSet() {
			swField.Set(reflect.ValueOf(w))
		}
	}
	if userInfo != nil {
		if field := clientValue.Elem().FieldByName("UserInfo"); field.IsValid() && field.CanSet() && field.Kind() == reflect.Pointer {
			field.Set(reflect.ValueOf(userInfo))
		}
	}

	if mapping == nil {
		return clientValue.Interface().(T)
	}

	structType := clientValue.Elem().Type()
	for i := 0; i < structType.NumField(); i++ {
		fieldType := structType.Field(i)
		fieldValue := clientValue.Elem().Field(i)

		if fieldType.Type.Kind() == reflect.String && fieldValue.CanSet() {
			key := strings.ToLower(fieldType.Name)
			if v, ok := mapping[key]; ok {
				fieldValue.SetString(v)
			}
		}
	}
	instance := clientValue.Interface().(T)
	if db, ok := any(instance).(*DouBaoChatCompletion); ok {
		db.douBaoClient = NewClient()
	}

	return instance
}

func insertAiMessage(userInfo *UserInfo, message, modelName string, isReason bool, processStep consts.ProcessStep) error {
	if userInfo == nil {
		return fmt.Errorf("userInfo is nil")
	}
	var stage *int
	for k, v := range stageMapping {
		if strings.Contains(strings.ToLower(string(processStep)), strings.ToLower(k)) {
			stage = &v
			break
		}
	}
	if stage == nil {
		return fmt.Errorf("no stage match")
	}
	aiMessage := &model.AiMessage{
		SessionID:     userInfo.SessionId,
		UserMessageID: userInfo.UserMessageId,
		UserID:        userInfo.UserId,
		ModelName:     modelName,
		Message:       message,
		IsReason:      isReason,
		Stage:         *stage,
	}
	aiMessageRepo := repository.AiMessageRepo[model.AiMessage]{}
	return aiMessageRepo.Create(userInfo.DB, aiMessage)
}

func writeSSEEvent(w io.Writer, flusher http.Flusher, data string) {
	if flusher == nil || w == nil {
		return
	}
	data = "{\"data\":" + data + "}"
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// filterHTML 从文本中提取完整的HTML代码
func filterHTML(html string) string {
	// 查找 <!DOCTYPE html> 的开始位置
	doctypeStart := strings.Index(html, "<!DOCTYPE html>")
	if doctypeStart == -1 {
		// 如果没有 DOCTYPE，尝试查找 <html> 标签
		htmlStart := strings.Index(html, "<html")
		if htmlStart == -1 {
			return html
		}
		doctypeStart = htmlStart
	}

	// 查找 </html> 的结束位置
	endTag := "</html>"
	endIdx := strings.Index(html[doctypeStart:], endTag)
	if endIdx == -1 {
		return html
	}

	// 计算结束位置
	endPos := doctypeStart + endIdx + len(endTag)

	// 提取完整的HTML文档
	htmlDoc := html[doctypeStart:endPos]

	return htmlDoc
}
