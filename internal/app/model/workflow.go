package model

import (
	"fmt"
)

type RawString string

type Workflow struct {
	Model
	SessionID  uint      `gorm:"not null;comment:关联会话ID" json:"sessionId"`
	MessageID  uint      `gorm:"not null;comment:关联消息ID" json:"messageId"`
	RootID     uint      `gorm:"default:0;comment:根工作流ID" json:"rootId"`
	ParentID   uint      `gorm:"default:0;comment:上一步工作流ID" json:"parentId"`
	Type       int       `gorm:"default:0;comment:步骤类型：1-提示词生成 2-GGB指令生成 3-HTML生成 4-HTML优化" json:"type"`
	Input      RawString `gorm:"type:text;comment:步骤输入内容" json:"input"`
	Output     RawString `gorm:"type:text;comment:步骤输出内容" json:"output"`
	ExecTimeMs int       `gorm:"default:0;comment:执行耗时(毫秒)" json:"execTimeMs"`
	Session    *Session  `gorm:"foreignKey:SessionID" json:"session"`
	Message    *Message  `gorm:"foreignKey:MessageID" json:"message"`
}

func (w *Workflow) TableName() string {
	return "tb_workflow"
}

func (r *RawString) Scan(value interface{}) error {
	if value == nil {
		*r = ""
		return nil
	}

	switch v := value.(type) {
	case []byte:
		*r = RawString(v) // 保持原始二进制数据
	case string:
		*r = RawString(v) // 如果是字符串也直接存储
	default:
		return fmt.Errorf("unsupported type: %T", value)
	}
	return nil
}
