package handler

import (
	"ggb_server/internal/app/model"
	"ggb_server/internal/app/schema"
	"ggb_server/internal/repository"
	"ggb_server/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
)

type User struct{}

func (u User) Register(c *gin.Context) {
	var req schema.RegisterRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, schema.ApiResponse{
			Success: false,
			Error:   "请求参数错误: " + err.Error(),
		})
		return
	}

	db := GetDB(c)
	userRepo := repository.NewUserRepository()

	// 检查用户名是否已存在
	existingUser, _ := userRepo.GetByUsername(db, req.Username)
	if existingUser != nil {
		c.JSON(http.StatusBadRequest, schema.ApiResponse{
			Success: false,
			Error:   "用户名已存在",
		})
		return
	}

	// 检查邮箱是否已存在（如果提供了邮箱）
	if req.Email != "" {
		existingUser, _ = userRepo.GetByEmail(db, req.Email)
		if existingUser != nil {
			c.JSON(http.StatusBadRequest, schema.ApiResponse{
				Success: false,
				Error:   "邮箱已存在",
			})
			return
		}
	}

	// 验证邀请码（如果提供）
	if req.InviteCode != "" {
		_, err := userRepo.GetByInviteCode(db, req.InviteCode)
		if err != nil {
			c.JSON(http.StatusBadRequest, schema.ApiResponse{
				Success: false,
				Error:   "邀请码无效",
			})
			return
		}
		// 可以在这里给邀请人增加奖励等逻辑
	}

	// 加密密码
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, schema.ApiResponse{
			Success: false,
			Error:   "密码加密失败",
		})
		return
	}

	// 生成邀请码
	inviteCode := utils.GenerateInviteCode()

	// 创建用户
	uid, _ := uuid.NewUUID()
	user := &model.User{
		UserId:           uid.String(),
		Username:         req.Username,
		Email:            req.Email,
		Password:         hashedPassword,
		InviteCode:       inviteCode,
		InvitedBy:        req.InviteCode,
		Status:           1,
		FreeMessageCount: 100,
		// LastLoginAt 字段不设置，保持为nil
	}

	err = userRepo.Create(db, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, schema.ApiResponse{
			Success: false,
			Error:   "用户创建失败",
		})
		return
	}

	c.JSON(http.StatusOK, schema.ApiResponse{
		Success: true,
		Message: "注册成功",
	})
}

func (u User) Login(c *gin.Context) {
	var req schema.LoginRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, schema.ApiResponse{
			Success: false,
			Error:   "请求参数错误: " + err.Error(),
		})
		return
	}

	db := GetDB(c)
	userRepo := repository.NewUserRepository()

	// 根据用户名查找用户
	user, err := userRepo.GetByUsername(db, req.Username)
	if err != nil {
		c.JSON(http.StatusUnauthorized, schema.ApiResponse{
			Success: false,
			Error:   "用户名或密码错误",
		})
		return
	}

	// 验证密码
	if !utils.CheckPassword(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, schema.ApiResponse{
			Success: false,
			Error:   "用户名或密码错误",
		})
		return
	}

	// 检查用户状态
	if user.Status != 1 {
		c.JSON(http.StatusForbidden, schema.ApiResponse{
			Success: false,
			Error:   "账户已被禁用",
		})
		return
	}

	// 生成JWT token
	token, err := utils.GenerateToken(user.UserId, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, schema.ApiResponse{
			Success: false,
			Error:   "Token生成失败",
		})
		return
	}

	// 更新登录信息
	userRepo.UpdateLoginInfo(db, user.ID)

	// 返回用户信息
	userInfo := schema.UserInfo{
		ID:               user.ID,
		UserId:           user.UserId,
		Username:         user.Username,
		Email:            user.Email,
		FreeMessageCount: user.FreeMessageCount,
	}

	c.JSON(http.StatusOK, schema.LoginResponse{
		AccessToken: token,
		User:        userInfo,
	})
}

func (u User) Logout(c *gin.Context) {
	// 这里可以实现token黑名单等逻辑
	c.JSON(http.StatusOK, schema.ApiResponse{
		Success: true,
		Message: "退出成功",
	})
}

func (u User) CheckLogin(c *gin.Context) {
	user, err := GetUserFromContext(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, schema.ApiResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	userInfo := schema.UserInfo{
		ID:               user.ID,
		Username:         user.Username,
		Email:            user.Email,
		FreeMessageCount: user.FreeMessageCount,
	}

	c.JSON(http.StatusOK, schema.ApiResponse{
		Success: true,
		Data:    userInfo,
	})
}

func (u User) ResetPassword(c *gin.Context) {
	c.JSON(http.StatusOK, schema.ApiResponse{
		Success: true,
		Message: "重置密码功能待实现",
	})
}

func (u User) SendVerificationCode(c *gin.Context) {
	c.JSON(http.StatusOK, schema.ApiResponse{
		Success: true,
		Message: "发送验证码功能待实现",
	})
}

func (u User) VerifyVerificationCode(c *gin.Context) {
	c.JSON(http.StatusOK, schema.ApiResponse{
		Success: true,
		Message: "验证验证码功能待实现",
	})
}

func (u User) GenerateInviteCode(c *gin.Context) {
	user, err := GetUserFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, schema.ApiResponse{
			Success: false,
			Error:   "用户不存在",
		})
		return
	}

	c.JSON(http.StatusOK, schema.ApiResponse{
		Success: true,
		Data: gin.H{
			"invite_code": user.InviteCode,
		},
	})
}
