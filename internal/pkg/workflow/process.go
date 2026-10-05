package workflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"ggb_server/internal/app/model"
	"ggb_server/internal/config"
	"ggb_server/internal/consts"
	"ggb_server/internal/pkg/workflow/aiModule"
	"ggb_server/internal/repository"
	"ggb_server/internal/utils"
	arkModel "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"gorm.io/gorm"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Process struct {
	UserMessage string
	ImgUrl      string
	Flusher     http.Flusher
	W           io.Writer
	Ctx         context.Context

	config consts.Config

	userInfo *aiModule.UserInfo
}

func NewProcess(userMessage string, imgUrl string, flusher http.Flusher, w io.Writer, ctx context.Context) *Process {
	return &Process{
		UserMessage: userMessage,
		ImgUrl:      imgUrl,
		Flusher:     flusher,
		W:           w,
		Ctx:         ctx,
	}
}

func (p *Process) StartProcess(db *gorm.DB, message *model.Message) error {
	if message == nil {
		return errors.New("invalid message")
	}
	p.userInfo = &aiModule.UserInfo{
		DB:            db,
		UserId:        message.UserID,
		SessionId:     message.SessionID,
		UserMessageId: message.ID,
	}

	rawRes, processedRes, err := p.Classify()
	workflow1, err := p.insertWorkFlow(nil, nil, consts.IntentRecognition, message.Message, rawRes, false)
	if err != nil {
		return err
	}

	elements, err := p.ExtractElementsStream(processedRes)
	if err != nil {
		return err
	}
	input, _ := json.Marshal(processedRes)
	workflow2, err := p.insertWorkFlow(&workflow1.ID, &workflow1.ID, consts.ExtractElement, string(input), elements, p.config.Extract.Skip)
	if err != nil {
		return err
	}

	filterEle := filterElements(elements)
	commands, err := p.GenerateGGB(filterEle)
	if err != nil {
		return err
	}
	workflow3, err := p.insertWorkFlow(&workflow1.ID, &workflow2.ID, consts.GenerateGGB, filterEle, commands, p.config.GenGGB.Skip)
	if err != nil {
		return err
	}

	filterCmd := filterCommands(commands)
	html, err := p.GenerateHTML(filterCmd)
	_, err = p.insertWorkFlow(&workflow1.ID, &workflow3.ID, consts.GenerateHTML, filterCmd, html, p.config.GenHTML.Skip)
	return err
}

func (p *Process) Classify() (string, map[string]string, error) {
	return p.doClassification()
}

func (p *Process) ExtractElementsStream(classifyRes map[string]string) (string, error) {
	p.config = p.lookUpClassification(classifyRes)
	if p.config.Extract.Skip {
		return "", nil
	}
	return p.doExtract(classifyRes)
}

func (p *Process) GenerateGGB(elements string) (string, error) {
	if p.config.GenGGB.Skip {
		return elements, nil
	}
	return p.doGenGGB(elements)
}

func (p *Process) GenerateHTML(command string) (string, error) {
	if p.config.GenHTML.Skip {
		return command, nil
	}
	res, err := p.doGenHTML(command)
	if err != nil {
		return "", err
	}
	return filterHTML(res), nil
}

func (p *Process) doClassification() (string, map[string]string, error) {
	imgUrl := p.ImgUrl
	if imgUrl != "" && (strings.Contains(imgUrl, "localhost") || strings.Contains(imgUrl, "127.0.0.1")) {
		imgPath := utils.ProcessUrl(imgUrl, config.Cfg.Static.Path)
		p.ImgUrl = imgPath
		file, err := os.Open(imgPath)
		if err != nil {
			log.Println("Error opening file error1: ", err)
			return "", nil, err
		}
		defer file.Close()
		fileContent, err := io.ReadAll(file)
		if err != nil {
			log.Println("Error reading file error: ", err)
			return "", nil, err
		}
		imgBase64 := fmt.Sprintf("data:image/%s;base64,%s", strings.TrimPrefix(strings.ToLower(filepath.Ext(imgPath)), "."), base64.StdEncoding.EncodeToString(fileContent))
		imgUrl = imgBase64
	}

	mapping := map[string]string{
		"model":                         string(consts.DouBaoSeed1V6),
		"message":                       p.UserMessage,
		strings.ToLower("imgUrl"):       imgUrl,
		strings.ToLower("processStep"):  string(consts.Classify),
		strings.ToLower("contentType"):  string(consts.Classify),
		strings.ToLower("thinkingType"): string(arkModel.ThinkingTypeEnabled),
	}

	client := aiModule.NewChatCompletionClient[*aiModule.DouBaoChatCompletion](mapping, p.Flusher, p.W, p.userInfo)
	res, err := client.ChatCompletion()
	if err != nil {
		log.Println(err)
		return "", nil, err
	}
	log.Println("\n[classify content]: ", res.Content)

	classifyRes, err := p.seekForClassificationRes(res)
	if err != nil {
		return "", nil, err
	}
	if classifyRes["类型"] == string(consts.Other) {
		return "", nil, errors.New("classification result error")
	}
	return res.Content, classifyRes, nil
}

func (p *Process) seekForClassificationRes(content aiModule.Content) (map[string]string, error) {
	re := regexp.MustCompile(`(?s)\{\s*"题目"\s*:\s*"[^"]*"\s*,\s*"类型"\s*:\s*"[^"]*"\s*\}`)
	matches := re.FindString(content.Content)
	if len(matches) == 0 {
		log.Println("classification result: ", matches)
		return nil, errors.New("classification result error")
	}
	log.Println("match result: ", matches)
	res, err := safeUnmarshalWithLatex(matches)
	if err != nil {
		log.Println("Error unmarshalling classification result error: ", err)
		return nil, err
	}

	problemType, ok := res["类型"]
	if !ok {
		log.Println("classification failed")
		return nil, errors.New("classification failed")
	}
	problem, ok := res["题目"]
	if !ok {
		log.Println("classification failed")
		return nil, errors.New("classification failed")
	}
	log.Printf("类型： %s , 题目： %s\n\n", problemType, problem)
	return res, nil
}

func (p *Process) lookUpClassification(classify map[string]string) consts.Config {
	switch classify["类型"] {
	case string(consts.G2D):
		return consts.ConfigMapping[consts.G2D]
	case string(consts.G3D):
		return consts.ConfigMapping[consts.G3D]
	case string(consts.Func):
		return consts.ConfigMapping[consts.Func]
	case string(consts.Knowledge):
		return consts.ConfigMapping[consts.Knowledge]
	case string(consts.Other):
		return consts.ConfigMapping[consts.Other]
	case string(consts.CommonGGBPlot):
		return consts.ConfigMapping[consts.CommonGGBPlot]
	default:
		return consts.ConfigMapping[consts.UnknownType]
	}
}

func (p *Process) doExtract(classify map[string]string) (string, error) {
	problem := classify["题目"]

	mapping := map[string]string{
		"model":                        string(consts.DouBaoSeed1V6),
		"message":                      problem,
		strings.ToLower("processStep"): string(p.config.Extract.ProcessStep),
		strings.ToLower("contentType"): string(aiModule.Reasoning),
	}
	if classify["类型"] == string(consts.G3D) {
		imgUrl := p.ImgUrl
		if p.ImgUrl != "" && (strings.Contains(imgUrl, "localhost") || strings.Contains(imgUrl, "127.0.0.1")) {
			url := utils.ProcessUrl(imgUrl, config.Cfg.Static.Path)
			file, err := os.Open(url)
			if err != nil {
				log.Println("Error opening file error2: ", err)
				return "", err
			}
			defer file.Close()
			fileContent, err := io.ReadAll(file)
			if err != nil {
				log.Println("Error reading file error: ", err)
				return "", err
			}
			imgBase64 := fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(fileContent))
			imgUrl = imgBase64
		}

		//mapping["model"] = string(consts.StepFuncChat1oTurbo)
		mapping[strings.ToLower("imgUrl")] = imgUrl
		/*		client := aiModule.NewChatCompletionClient[*aiModule.StepFunChatCompletion](mapping, p.Flusher, p.W, p.userInfo)

				res, err := client.ChatCompletionStream()
				if err != nil {
					log.Println("error occurred when extract: ", err)
					return "", err
				}
				return res.Content, err*/
	}

	mapping[strings.ToLower("thinkingType")] = string(arkModel.ThinkingTypeEnabled)
	var client aiModule.ChatCompletionInterface
	client = aiModule.NewChatCompletionClient[*aiModule.DouBaoChatCompletion](mapping, p.Flusher, p.W, p.userInfo)

	res, err := client.ChatCompletionStream()
	if err != nil {
		log.Printf("type: %v, step: %v, content: %v\n", res.Type, res.Step, res.Content)
		return "", err
	}
	log.Println("\n[extract content]: ", res.Content)

	return res.Content, nil
}

func (p *Process) doGenGGB(elements string) (string, error) {
	reader := strings.Builder{}
	reader.WriteString("数学元素: \n")
	reader.WriteString(elements)
	mapping := map[string]string{
		"model":                        string(consts.DeepSeekChat),
		"message":                      reader.String(),
		strings.ToLower("processStep"): string(p.config.GenGGB.ProcessStep),
	}
	client := aiModule.NewChatCompletionClient[*aiModule.ChatCompletionClient](mapping, p.Flusher, p.W, p.userInfo)
	res, err := client.ChatCompletion()
	if err != nil {
		log.Printf("type: %v, step: %v, content: %v\n", res.Type, res.Step, res.Content)
		return "", err
	}
	log.Print("\n[gen ggb content]: ", res.Content)
	return res.Content, nil
}

func (p *Process) doGenHTML(command string) (string, error) {
	mapping := map[string]string{
		"model":                        string(consts.DeepSeekChat),
		"message":                      command,
		strings.ToLower("processStep"): string(p.config.GenHTML.ProcessStep),
	}
	client := aiModule.NewChatCompletionClient[*aiModule.ChatCompletionClient](mapping, p.Flusher, p.W, p.userInfo)
	res, err := client.ChatCompletion()
	if err != nil {
		log.Printf("type: %v, step: %v, content: %v\n", res.Type, res.Step, res.Content)
		return "", err
	}
	log.Println("\n[gen html content]: ", res.Content)
	return res.Content, nil
}

func filterElements(elements string) string {
	startTag := "<element_contents>"
	endTag := "</element_contents>"

	startIdx := strings.Index(elements, startTag)
	if startIdx == -1 {
		return elements
	}

	endIdx := strings.Index(elements[startIdx:], endTag)
	if endIdx == -1 {
		return elements
	}

	contentStart := startIdx + len(startTag)
	contentEnd := startIdx + endIdx
	return elements[contentStart:contentEnd]
}

func filterCommands(elements string) string {
	startTag := "<ggb_commands>"
	endTag := "</ggb_commands>"

	startIdx := strings.Index(elements, startTag)
	if startIdx == -1 {
		return elements
	}

	endIdx := strings.Index(elements[startIdx:], endTag)
	if endIdx == -1 {
		return elements
	}

	contentStart := startIdx + len(startTag)
	contentEnd := startIdx + endIdx
	return elements[contentStart:contentEnd]
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

func (p *Process) insertWorkFlow(rootId, parentId *uint, workflowType consts.WorkFlowType, input, output string, skip bool) (*model.Workflow, error) {
	if skip {
		tmp := uint(0)
		if parentId == nil {
			parentId = &tmp
		}
		return &model.Workflow{
			Model: model.Model{
				ID: *parentId,
			},
		}, nil
	}
	if p.userInfo == nil {
		return nil, fmt.Errorf("userInfo is nil")
	}
	workflow := &model.Workflow{
		SessionID: p.userInfo.SessionId,
		MessageID: p.userInfo.UserMessageId,
		Type:      int(workflowType),
		Input:     model.RawString(input),
		Output:    model.RawString(output),
	}
	if rootId != nil {
		workflow.RootID = *rootId
	}
	if parentId != nil {
		workflow.ParentID = *parentId
	}
	err := repository.NewWorkflowRepository[model.Workflow]().Create(p.userInfo.DB, workflow)
	return workflow, err
}

func safeUnmarshalWithLatex(matches string) (map[string]string, error) {
	re := regexp.MustCompile(`"([^"\\]*(\\.[^"\\]*)*)"`)

	safeJSON := re.ReplaceAllStringFunc(matches, func(s string) string {
		content := s[1 : len(s)-1]
		content = strings.ReplaceAll(content, `\`, `\\`)
		return `"` + content + `"`
	})

	res := map[string]string{}
	err := json.Unmarshal([]byte(safeJSON), &res)
	return res, err
}
