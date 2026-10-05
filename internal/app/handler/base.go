package handler

import (
	"errors"
	"ggb_server/internal/app/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetDB(c *gin.Context) *gorm.DB {
	return c.MustGet("db").(*gorm.DB)
}

func GetUserIdFromContext(c *gin.Context) (string, error) {
	val, exist := c.Get("userId")
	if !exist {
		return "", errors.New("userId not exist or no token provided")
	}
	return val.(string), nil
}

func GetUserFromContext(c *gin.Context) (*model.User, error) {
	key, exist := c.Get("user")
	if !exist || key == nil {
		return nil, errors.New("user not exist or no token provided")
	}
	return key.(*model.User), nil
}
