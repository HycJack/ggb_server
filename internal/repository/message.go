package repository

import (
	"ggb_server/internal/app/model"
	"gorm.io/gorm"
)

type MessageRepository[T any] interface {
	Generic[T]
	GetBySessionID(db *gorm.DB, sessionID uint) ([]model.Message, error)
	GetLastMessage(db *gorm.DB, sessionID uint) (*model.Message, error)
	BatchCreate(db *gorm.DB, messages []model.Message) error
	GetAiMessage(db *gorm.DB) ([]model.Message, error)
}

type MessageRepo[T any] struct {
	GenericImpl[T]
}

func NewMessageRepository[T any]() MessageRepository[T] {
	return &MessageRepo[T]{
		GenericImpl[T]{},
	}
}

func (r *MessageRepo[T]) GetBySessionID(db *gorm.DB, sessionID uint) ([]model.Message, error) {
	var messages []model.Message
	err := db.Where("session_id = ? AND is_del = 0", sessionID).
		//Offset((page - 1) * pageSize).
		//Limit(pageSize).
		Preload("AiMessages", func(db *gorm.DB) *gorm.DB {
			return db.Where("stage IN (1, 3)").Order("created_at ASC, is_reason DESC")
		}).
		Preload("Resources").
		Order("created_at DESC").
		Find(&messages).Error
	return messages, err
}

func (r *MessageRepo[T]) GetLastMessage(db *gorm.DB, sessionID uint) (*model.Message, error) {
	var message model.Message
	err := db.Where("session_id = ? AND is_del = 0", sessionID).
		Order("created_at DESC").
		First(&message).Error
	return &message, err
}

func (r *MessageRepo[T]) BatchCreate(db *gorm.DB, messages []model.Message) error {
	return db.CreateInBatches(messages, 100).Error
}

func (r *MessageRepo[T]) GetAiMessage(db *gorm.DB) ([]model.Message, error) {
	var messages []model.Message
	err := db.Preload("AiMessages").Preload("Resources").Find(&messages).Error
	return messages, err
}
