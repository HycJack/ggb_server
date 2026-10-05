package repository

type AiMessageRepository[T any] interface {
	Generic[T]
	//GetBySessionID(db *gorm.DB, sessionID int64, page, pageSize int) ([]model.Message, error)
	//GetLastMessage(db *gorm.DB, sessionID int64) (*model.Message, error)
	//BatchCreate(db *gorm.DB, messages []model.Message) error
}

type AiMessageRepo[T any] struct {
	GenericImpl[T]
}

func NewAiMessageRepository[T any]() AiMessageRepository[T] {
	return &MessageRepo[T]{
		GenericImpl[T]{},
	}
}
