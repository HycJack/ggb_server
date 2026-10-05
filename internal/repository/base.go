package repository

import "gorm.io/gorm"

type Generic[T any] interface {
	Create(db *gorm.DB, entity *T) error
	GetById(db *gorm.DB, id int) (*T, error)
	Update(db *gorm.DB, entity *T) error
	List(db *gorm.DB, page, pageSize int) ([]T, error)
}

type GenericImpl[T any] struct{}

func (g GenericImpl[T]) Create(db *gorm.DB, entity *T) error {
	return db.Create(entity).Error
}

func (g GenericImpl[T]) GetById(db *gorm.DB, id int) (*T, error) {
	var entity T
	err := db.First(&entity, id).Error
	return &entity, err
}

func (g GenericImpl[T]) Update(db *gorm.DB, entity *T) error {
	return db.Save(entity).Error
}

func (g GenericImpl[T]) List(db *gorm.DB, page, pageSize int) ([]T, error) {
	var entities []T
	err := db.Limit(pageSize).Offset((page - 1) * pageSize).Find(&entities).Error
	return entities, err
}
