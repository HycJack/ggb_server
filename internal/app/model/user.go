package model

import "time"

type User struct {
	Model
	UserId           string     `json:"user_id"`
	Username         string     `gorm:"size:50;uniqueIndex;not null;comment:用户名" json:"username"`
	Email            string     `gorm:"size:100;uniqueIndex;comment:邮箱" json:"email"`
	Password         string     `gorm:"size:255;not null;comment:密码" json:"-"`
	InviteCode       string     `gorm:"size:20;comment:邀请码" json:"inviteCode"`
	InvitedBy        string     `gorm:"size:20;comment:被谁邀请" json:"invitedBy"`
	LastLoginAt      *time.Time `gorm:"comment:最后登录时间" json:"lastLoginAt"`
	LoginCount       int        `gorm:"default:0;comment:登录次数" json:"loginCount"`
	Status           int        `gorm:"default:1;comment:状态：0-禁用 1-正常" json:"status"`
	FreeMessageCount int        `gorm:"default:100;comment:剩余免费消息额度" json:"freeMessageCount"`
	IsDel            bool       `gorm:"default:false" json:"isDel"`
}

func (u *User) TableName() string {
	return "users"
}
