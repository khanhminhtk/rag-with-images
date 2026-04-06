package util

import (
	"fmt"
	"log"
)

func Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Fatal(withCaller(normalizeMessage(msg), 2))
}
