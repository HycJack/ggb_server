package api

import (
	"ggb_server/internal/app/handler"
	"ggb_server/internal/config"
	"ggb_server/internal/middleware"
	"ggb_server/internal/pkg/database"
	"ggb_server/internal/utils"
	"github.com/gin-gonic/gin"
	"os"
	"path/filepath"
)

var (
	AIChatAPI handler.AiChat
	LoginAPI  handler.User
)

func AddPath(e *gin.Engine) {
	rootPath, _ := utils.FindRootPath()
	uploadDir := filepath.Join(rootPath, config.Cfg.Static.Path)
	err := os.MkdirAll(uploadDir, os.ModePerm)
	if err != nil {
		return
	}
	e.Static(config.Cfg.Static.Path, uploadDir)
	e.LoadHTMLGlob(filepath.Join(rootPath, "static/ui/*.html"))
	e.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", gin.H{})
	})
	e.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r := e.Group("/api")
	aiChatModule(r)
	loginModule(r)
	userModule(r)
}

func AddMiddleware(e *gin.Engine) {
	e.Use(middleware.SetHost())
	e.Use(middleware.CORSMiddleware())
	e.Use(middleware.Logger())
	e.Use(middleware.Recovery())
	e.Use(middleware.DBMiddleware(database.GetDB()))
	e.Use(middleware.JWTAuthMiddleware())
}

func aiChatModule(r *gin.RouterGroup) {
	r.POST("/upload", AIChatAPI.Upload)
	r.POST("/v2/chat", AIChatAPI.Chat)
	r.POST("/conversations", AIChatAPI.CreateConversation)
	r.GET("/conversations", AIChatAPI.GetConversations)
	r.DELETE("/conversations/:id", AIChatAPI.DeleteConversation)
	r.GET("/conversations/:id", AIChatAPI.GetConversation)
}

func loginModule(r *gin.RouterGroup) {
	r.POST("/login", LoginAPI.Login)
	r.POST("/register", LoginAPI.Register)
	r.POST("/logout", LoginAPI.Logout)
	r.POST("/check-login", LoginAPI.CheckLogin)
	r.POST("/reset-password", LoginAPI.ResetPassword)
	r.POST("/send-verification-code", LoginAPI.SendVerificationCode)
	r.POST("/verify-verification-code", LoginAPI.VerifyVerificationCode)
	r.POST("/generate-invite-code", LoginAPI.GenerateInviteCode)

}

func userModule(r *gin.RouterGroup) {
	r = r.Group("/user")
	{

	}
}
