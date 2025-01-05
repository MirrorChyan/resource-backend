package rand

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func TempDirName() (string, error) {
	now := time.Now()

	randBytes := make([]byte, 8)
	_, err := rand.Read(randBytes)
	if err != nil {
		return "", err
	}
	randStr := hex.EncodeToString(randBytes)

	return fmt.Sprintf("%s%s", randStr, now.Format("20060102150405")), nil
}
