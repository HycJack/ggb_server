package app

import (
	"context"
	"errors"
	"ggb_server/internal/app/api"
	"ggb_server/internal/app/model"
	"ggb_server/internal/config"
	"ggb_server/internal/pkg/database"
	"ggb_server/internal/pkg/glog"
	"ggb_server/internal/utils"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func Start() {
	rootPath, _ := utils.FindRootPath()

	// JWT 必须在服务起来之前初始化，空密钥比无认证更危险
	utils.InitJWTAuth(config.Cfg.JWT.Secret, config.Cfg.JWT.Expire)

	if err := database.InitDB(config.Cfg.Database.GetDSN(), config.Cfg.Database.MaxIdleConns, config.Cfg.Database.MaxOpenConns); err != nil {
		log.Fatalf("init db err: %v", err)
	}
	// 建缺失的表（幂等）；已有库不受影响
	if err := database.AutoMigrate(
		&model.User{}, &model.Session{}, &model.Message{},
		&model.AiMessage{}, &model.Resource{}, &model.Workflow{},
	); err != nil {
		log.Fatalf("auto migrate: %v", err)
	}

	logPath := config.Cfg.Log.FilePath
	if logPath == "" {
		logPath = filepath.Join(rootPath, "logs/app.log")
	} else if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(rootPath, logPath)
	}
	glog.InitLogger(logPath)

	switch config.Cfg.Server.Mode {
	case "prod":
		gin.SetMode(gin.ReleaseMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	e := gin.New()
	api.AddMiddleware(e)
	api.AddPath(e)

	srv := &http.Server{
		Addr:              ":" + config.Cfg.Server.Port,
		Handler:           e,
		ReadHeaderTimeout: 30 * time.Second, // 只管读完请求头；长响应由 WriteTimeout 保证
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       30 * time.Minute,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	log.Println("ggb server started on :" + config.Cfg.Server.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second) //设置请求10min超时
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	log.Println("Server exiting")
}
