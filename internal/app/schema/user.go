package schema

type LoginRequest struct {
	Username string `form:"username" binding:"required" json:"username"`
	Password string `form:"password" binding:"required" json:"password"`
}

type RegisterRequest struct {
	Username   string `form:"username" binding:"required" json:"username"`
	Password   string `form:"password" binding:"required" json:"password"`
	Email      string `form:"email" binding:"omitempty,email" json:"email,omitempty"`
	InviteCode string `form:"invite_code" json:"invite_code,omitempty"`
}

type LoginResponse struct {
	AccessToken string   `json:"access_token"`
	User        UserInfo `json:"user"`
}

type UserInfo struct {
	ID               uint   `json:"id"`
	UserId           string `json:"userId"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	FreeMessageCount int    `json:"freeMessageCount"`
}

type ApiResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}
