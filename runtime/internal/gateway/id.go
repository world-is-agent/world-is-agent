package gateway

import (
	"fmt"
	"time"
)

func newMessageID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
