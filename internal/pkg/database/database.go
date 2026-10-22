package database

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	dbInstance *gorm.DB
	once       sync.Once
)

func InitDB(dsn string, maxIdleConns, maxOpenConns int) error {
	var err error

	once.Do(func() {
		newLogger := logger.New(
			log.New(log.Writer(), "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold: 10 * time.Second,
				LogLevel:      logger.Error,
				Colorful:      true,
			})
		dbInstance, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger: newLogger,
			NowFunc: func() time.Time {
				return time.Now().UTC()
			},
		})

		if err != nil {
			return
		}

		sqlDB, dbErr := dbInstance.DB()
		if dbErr != nil {
			err = fmt.Errorf("get underlying sql.DB: %w", dbErr)
			return
		}
		if maxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(maxIdleConns)
		}
		if maxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(maxOpenConns)
		}
		sqlDB.SetConnMaxLifetime(time.Hour)
	})
	log.Println("connected to database")
	return err
}

func GetDB() *gorm.DB {
	return dbInstance
}

func CloseDB() {
	if dbInstance != nil {
		sqlDB, err := dbInstance.DB()
		if err != nil {
			log.Println("error getting db instance: ", err)
			return
		}
		sqlDB.Close()
		log.Println("db closed")
	}
}

func AutoMigrate(models ...interface{}) error {
	return GetDB().AutoMigrate(models...)
}
