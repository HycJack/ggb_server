package handler

import (
	"encoding/json"
	"fmt"
	"ggb_server/internal/app/model"
	"ggb_server/internal/app/schema"
	"ggb_server/internal/config"
	"ggb_server/internal/consts"
	"ggb_server/internal/pkg/workflow"
	"ggb_server/internal/pkg/workflow/aiModule"
	"ggb_server/internal/repository"
	"ggb_server/internal/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

type AiChat struct{}

func (a AiChat) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{
			"message": err.Error(),
		})
	}
	fileExt := filepath.Ext(file.Filename)
	newFileName := fmt.Sprintf("%d%s", time.Now().UnixNano(), fileExt)
	rootPath, _ := utils.FindRootPath()
	savePath := filepath.Join(rootPath, config.Cfg.Static.Path, newFileName)
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		c.JSON(500, gin.H{
			"message": err.Error(),
		})
	}
	c.JSON(200, gin.H{
		"imageUrl": fmt.Sprintf("%s/%s/%s", consts.URLPrefix, config.Cfg.Static.Path, newFileName),
	})
}

func (a AiChat) Chat(c *gin.Context) {
	var (
		chatRequest schema.ChatRequest
	)

	if err := c.ShouldBind(&chatRequest); err != nil {
		c.JSON(400, gin.H{
			"message": err.Error(),
		})
		return
	}

	log.Printf("chatRequest: %+v", chatRequest)

	user, err := GetUserFromContext(c)
	if err != nil {
		c.JSON(400, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	message, err := insertMessage(GetDB(c), chatRequest, user)
	if err != nil {
		c.JSON(500, gin.H{
			"error": err,
		})
	}

	flusher := c.Writer.(http.Flusher)
	w := c.Writer
	c.Writer.WriteHeader(http.StatusOK)
	processor := workflow.NewProcess(chatRequest.Message, chatRequest.ImageUrl, flusher, w, c.Request.Context())
	err = processor.StartProcess(GetDB(c), message)
	if err != nil {
		content := aiModule.Content{
			Type:    "failed_text",
			Step:    "failed",
			Content: "暂时无法处理这类请求，请换一个问题",
		}
		data, _ := json.Marshal(content)
		resData := "{\"data\":" + string(data) + "}"
		fmt.Fprintf(w, "data: %s\n\n", resData)
		flusher.Flush()
		log.Println("[error] occurred when processing: ", err)
	}
	flusher.Flush()
}

func (a AiChat) CreateConversation(c *gin.Context) {
	var req schema.CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误: " + err.Error(),
		})
		return
	}

	user, err := GetUserFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	db := GetDB(c)
	sessionRepo := repository.NewSessionRepository[model.Session]()

	// 创建新的session
	session := &model.Session{
		Title:            req.Title,
		UserID:           user.UserId,
		MessageCount:     0,
		FreeMessageCount: 100, // 默认免费消息额度
		IsDel:            0,
	}

	err = sessionRepo.Create(db, session)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建对话失败",
		})
		return
	}
	resultConversation := &schema.ConversationInfo{
		ID:        session.ID,
		Title:     session.Title,
		UpdatedAt: session.UpdatedAt.Format("2006-01-02 15:04:05"),
		CreatedAt: session.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	c.JSON(http.StatusOK, resultConversation)
}

func (a AiChat) GetConversations(c *gin.Context) {
	user, err := GetUserFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 获取分页参数
	page := 1
	pageSize := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr := c.Query("pageSize"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	db := GetDB(c)
	sessionRepo := repository.NewSessionRepository[model.Session]()

	// 获取用户的对话列表
	sessions, err := sessionRepo.GetByUserID(db, user.UserId, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取对话列表失败",
		})
		return
	}

	// 获取总数
	_, err = sessionRepo.CountByUserID(db, user.UserId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取对话总数失败",
		})
		return
	}

	// 转换为响应格式
	//conversations := make([]schema.ConversationInfo, len(sessions))
	//for i, session := range sessions {
	//	conversations[i] = schema.ConversationInfo{
	//		ID:        session.ID,
	//		Title:     session.Title,
	//		CreatedAt: session.CreatedAt.Format("2006-01-02 15:04:05"),
	//		UpdatedAt: session.UpdatedAt.Format("2006-01-02 15:04:05"),
	//	}
	//}

	c.JSON(http.StatusOK, gin.H{
		"userId":        user.UserId,
		"userPK":        user.ID,
		"conversations": sessions,
	})
}

func (a AiChat) GetConversation(c *gin.Context) {
	// 获取对话ID
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "对话ID不能为空",
		})
		return
	}
	cId, err := strconv.Atoi(conversationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	db := GetDB(c)
	session, err := repository.NewSessionRepository[model.Session]().GetById(db, cId)
	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	messages, err := repository.NewMessageRepository[model.Message]().GetBySessionID(db, session.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": map[string]any{
			"message": messages,
		},
	})
}

func (a AiChat) DeleteConversation(c *gin.Context) {
	userID, err := GetUserIdFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 获取对话ID
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "对话ID不能为空",
		})
		return
	}

	id, err := strconv.Atoi(conversationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	db := GetDB(c)
	sessionRepo := repository.NewSessionRepository[model.Session]()

	// 获取对话详情
	session, err := sessionRepo.GetById(db, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "对话不存在",
		})
		return
	}

	// 检查权限（只能删除自己的对话）
	if session.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "无权限删除此对话",
		})
		return
	}

	// 软删除对话
	err = sessionRepo.SoftDelete(db, uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除对话失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "对话删除成功",
	})
}

func insertMessage(db *gorm.DB, chatRequest schema.ChatRequest, user *model.User) (*model.Message, error) {
	message := &model.Message{
		ParentID:  chatRequest.ParentId,
		SessionID: chatRequest.SessionId,
		UserID:    user.UserId,
		Message:   chatRequest.Message,
		Identity:  int(consts.User),
	}
	messageRepo := repository.NewMessageRepository[model.Message]()
	if err := messageRepo.Create(db, message); err != nil {
		return nil, err
	}
	if chatRequest.ImageUrl != "" {
		resource := model.Resource{
			SessionID: chatRequest.SessionId,
			MessageID: message.ID,
			Type:      int(consts.Image),
			URL:       utils.ProcessUrl(chatRequest.ImageUrl, config.Cfg.Static.Path),
		}
		return message, repository.NewResourceRepository[model.Resource]().Create(db, &resource)
	}
	return message, nil
}
