package model

type Message struct {
	Model
	ParentID   uint         `gorm:"default:0;comment:父消息ID" json:"parentId"`
	SessionID  uint         `gorm:"not null;comment:关联的会话ID" json:"sessionId"`
	UserID     string       `gorm:"type:char(36);not null" json:"userId"`
	Message    string       `gorm:"type:text;comment:消息正文内容" json:"message"`
	Identity   int          `gorm:"default:0;comment:消息身份：0-用户 1-模型" json:"identity"`
	ExecTime   int          `gorm:"default:0;comment:处理耗时(毫秒)" json:"execTime"`
	Status     int          `gorm:"default:0;comment:状态：0-正常 1-撤回 2-删除" json:"status"`
	Data       JSON         `gorm:"type:json;comment:扩展数据" json:"data"`
	IsDel      bool         `gorm:"default:0" json:"isDel"`
	Session    *Session     `gorm:"foreignKey:SessionID" json:"session"`
	Workflows  []*Workflow  `gorm:"foreignKey:MessageID" json:"workflow"`
	Resources  []*Resource  `gorm:"foreignKey:MessageID" json:"resource"`
	AiMessages []*AiMessage `gorm:"foreignKey:UserMessageID;references:ID" json:"aiMessage"`
}

func (m *Message) TableName() string {
	return "tb_message"
}

type MessageData struct {
}

func (m *Message) GetData() (*MessageData, error) {
	var data MessageData
	err := m.Data.ToStruct(&data)
	return &data, err
}

func (m *Message) SetData(data *MessageData) error {
	return m.Data.FromStruct(data)
}
