package model

import (
	"ggb_server/internal/config"
	"ggb_server/internal/consts"
	"ggb_server/internal/utils"
	"gorm.io/gorm"
)

type Resource struct {
	Model
	SessionID uint     `gorm:"not null;comment:关联会话ID" json:"sessionId"`
	MessageID uint     `gorm:"not null;comment:关联消息ID" json:"messageId"`
	Type      int      `gorm:"default:0;comment:资源类型：1-图片 2-视频 3-HTML文件 4-其他" json:"type"`
	URL       string   `gorm:"type:text;comment:资源存储路径" json:"url"`
	Data      JSON     `gorm:"type:json;comment:扩展数据" json:"data"`
	IsDel     bool     `gorm:"default:0" json:"isDel"`
	Session   *Session `gorm:"foreignKey:SessionID" json:"session"`
	Message   *Message `gorm:"foreignKey:MessageID" json:"message"`
}

func (r *Resource) TableName() string {
	return "tb_resource"
}

func (r *Resource) AfterFind(db *gorm.DB) error {
	url := utils.RecoverUrl(r.URL, consts.URLPrefix, config.Cfg.Static.Path)
	r.URL = url
	return nil
}

type ResourceData struct {
	Size     uint   `json:"size"`     // 文件大小
	Width    int    `json:"width"`    // 图片宽度
	Height   int    `json:"height"`   // 图片高度
	Duration int    `json:"duration"` // 视频时长(秒)
	Format   string `json:"format"`   // 文件格式
}

func (r *Resource) GetData() (*ResourceData, error) {
	var data ResourceData
	err := r.Data.ToStruct(&data)
	return &data, err
}

func (r *Resource) SetData(data *ResourceData) error {
	return r.Data.FromStruct(data)
}
