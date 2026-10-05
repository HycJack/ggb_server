package model

type AiMessage struct {
	Model
	SessionID     uint   `gorm:"not null;comment:关联的会话ID" json:"sessionId"`
	UserMessageID uint   `gorm:"not null;comment:关联的用户消息" json:"userMessageId"`
	UserID        string `gorm:"type:char(36);not null" json:"userId"`
	Message       string `gorm:"type:text;comment:消息正文内容" json:"message"`
	ExecTime      int    `gorm:"default:0;comment:处理耗时(毫秒)" json:"execTime"`
	ModelName     string `json:"modelName"`
	Stage         int    `json:"stage"`
	IsReason      bool   `json:"isReason"`
	Status        bool   `gorm:"default:0;comment:状态：0-正常 1-撤回 2-删除" json:"status"`
	Data          JSON   `gorm:"type:json;comment:扩展数据" json:"data"`
	IsDel         bool   `gorm:"default:false" json:"isDel"`

	Session     Session `gorm:"foreignKey:SessionID" json:"-"`
	UserMessage Message `gorm:"foreignKey:UserMessageID" json:"-"`
}

func (m *AiMessage) TableName() string {
	return "tb_ai_message"
}
