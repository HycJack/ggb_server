package middleware

import (
	"ggb_server/internal/app/handler"
	"ggb_server/internal/consts"
	"ggb_server/internal/pkg/glog"
	"ggb_server/internal/repository"
	"ggb_server/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

var whiteList = map[string]bool{
	"/health" : true,
	"/api/register": true,
	"/api/login":    true,
	"/static":       true,
}

func SetHost() gin.HandlerFunc {
	return func(c *gin.Context) {
		if consts.URLPrefix != "" {
			c.Next()
		}
		protocol := "http"
		if c.Request.TLS != nil {
			protocol = "https"
		}
		consts.URLPrefix = protocol + "://" + c.Request.Host
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if whiteList[c.Request.URL.Path] {
			c.Next()
			return
		}
		if c.Request.Method == http.MethodGet && strings.Contains(c.Request.URL.Path, "/static/") {
			c.Next()
			return
		}

		tokenString := c.GetHeader("Authorization")

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "Authorization header required"})
			c.Abort()
			return
		}

		if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
			tokenString = tokenString[7:]
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "Invalid token format, expected 'Bearer <token>'"})
			c.Abort()
			return
		}

		claims, err := utils.ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "Invalid or expired token: " + err.Error()})
			c.Abort()
			return
		}

		if claims.ExpiresAt < time.Now().Unix() {
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "Expired token"})
			c.Abort()
			return
		}

		db := handler.GetDB(c)
		userRepo := repository.NewUserRepository()
		user, err := userRepo.GetByUserId(db, claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "user doesn't exist"})
			c.Abort()
			return
		}

		c.Set("userId", user.UserId)
		c.Set("username", user.Username)
		c.Set("user", user)

		c.Next()
	}
}

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				requestID, _ := c.Get("RequestID")

				glog.WithRequestID(requestID.(string)).Error("panic recovered",
					zap.Any("error", err),
					zap.String("stack", string(debug.Stack())),
				)

				c.AbortWithStatusJSON(500, gin.H{
					"code":      500,
					"msg":       "Internal Server Error",
					"data":      nil,
					"requestID": requestID,
				})
			}
		}()
		c.Next()
	}
}

func DBMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	}
}

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		requestID := generateRequestID()
		c.Set("requestID", requestID)
		c.Next()

		cost := time.Since(start)
		glog.WithRequestID(requestID).Info(path,
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("user-agent", c.Request.UserAgent()),
			zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()),
			zap.Duration("cost", cost),
		)
	}
}

func generateRequestID() string {
	return "req_" + time.Now().Format("20060102150405")
}
