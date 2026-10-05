package model

type Session struct {
	Model
	Title            string      `gorm:"size:128;comment:会话标题（可自动生成）" json:"title"`
	UserID           string      `gorm:"type:char(36);not null;comment:用户ID" json:"userId"`
	MessageCount     int         `gorm:"default:0;comment:总消息数" json:"messageCount"`
	FreeMessageCount int         `gorm:"default:0;comment:剩余免费消息额度" json:"freeMessageCount"`
	Data             JSON        `gorm:"type:json;comment:扩展数据" json:"data"`
	IsDel            int8        `gorm:"default:0;comment:0-正常 1-删除" json:"isDel"`
	Messages         []*Message  `gorm:"foreignKey:SessionID" json:"message"`
	Workflows        []*Workflow `gorm:"foreignKey:SessionID" json:"workflow"`
	Resources        []*Resource `gorm:"foreignKey:SessionID" json:"resource"`
}

func (s *Session) TableName() string {
	return "tb_session"
}

type SessionData struct {
	InputTypes   []string `json:"inputType"`    // ["text","image"]
	LastPreview  string   `json:"lastPreview"`  // 缩略图URL
	ModelVersion string   `json:"modelVersion"` // 使用模型
}

func (s *Session) GetData() (*SessionData, error) {
	var data SessionData
	err := s.Data.ToStruct(&data)
	return &data, err
}

// SetData 设置SessionData
func (s *Session) SetData(data *SessionData) error {
	return s.Data.FromStruct(data)
}
