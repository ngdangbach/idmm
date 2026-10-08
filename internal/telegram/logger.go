package telegram

import (
	"fmt"
	"time"
)

// Logf in ra thông điệp kèm timestamp chuẩn [YYYY-MM-DD HH:mm:ss]
func Logf(format string, a ...interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("["+ts+"] "+format, a...)
}

// Log in ra các tham số kèm timestamp chuẩn [YYYY-MM-DD HH:mm:ss]
func Log(a ...interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Print("[" + ts + "] ")
	fmt.Println(a...)
}
