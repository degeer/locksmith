//go:build debug
// +build debug

package main

import (
	"log"
	"os"
)

func init() {
	f, err := os.OpenFile(logFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, logFilePermissions)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	log.SetOutput(f)
	log.Println("-------------------------------------------------")
	log.Println("Locksmith application started")
}
