package cursor

import (
	"encoding/base64"
	"github.com/bytedance/sonic"
	"time"
)

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func (c *Cursor) Encode() (string, error) {
	jsonData, err := sonic.Marshal(c)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(jsonData), nil
}

func Decode(cursorStr string) (*Cursor, error) {
	jsonData, err := base64.URLEncoding.DecodeString(cursorStr)
	if err != nil {
		return nil, err
	}

	var cursor Cursor
	err = sonic.Unmarshal(jsonData, &cursor)
	if err != nil {
		return nil, err
	}

	return &cursor, nil
}
