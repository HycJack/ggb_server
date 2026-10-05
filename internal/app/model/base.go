package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type Model struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

type JSON json.RawMessage

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
	}
	s, ok := value.([]byte)
	if !ok {
		return errors.New("invalid JSON")
	}
	*j = append((*j)[0:0], s...)
	return nil
}

func (j *JSON) Value() (driver.Value, error) {
	if len(*j) == 0 {
		return nil, nil
	}
	return []byte(*j), nil
}

func (j *JSON) MarshalJSON() ([]byte, error) {
	if *j == nil { //表中data为null []byte为nil 需要返回一个json合法值
		return []byte("{}"), nil
	}
	return *j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if *j == nil {
		return errors.New("null input")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

func (j *JSON) ToStruct(v interface{}) error {
	return json.Unmarshal(*j, v)
}

func (j *JSON) FromStruct(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	*j = data
	return nil
}
