package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const (
	SHARED_DRIVE_ID_ENV_VAR = "SHARED_DRIVE_ID"
)

var driveID = os.Getenv(SHARED_DRIVE_ID_ENV_VAR)

var client = &http.Client{}
var failedRecords sync.Map

func init() {

	log.SetFlags(log.LstdFlags | log.Lshortfile)

}

func cleanup() {
	log.Printf("Cleaning up...")

	failed := make(map[string]string)
	failedRecords.Range(func(k, v interface{}) bool {
		failed[k.(string)] = v.(string)
		return true
	})

	b, _ := json.MarshalIndent(failed, " ", " ")
	log.Printf("Failed records:\n%s", string(b))

	os.Exit(0)
}

func main() {

	defer cleanup()

	ctx := context.Background()

	if driveID == "" {
		log.Fatalf("Missing %s env var", SHARED_DRIVE_ID_ENV_VAR)
	}

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		log.Println("Caught shutdown signal, cleaning up")
		cleanup()
	}()

	driveService, err := NewDrive(ctx, driveID)

	if err != nil {
		log.Println("Error initializing drive service:", err)
		return
	}

	for id, name := range recordTypeMap {

		count, err := syncRecords(ctx, driveService, id)
		if err != nil {
			log.Println("Error syncing records:", err)
			return
		}
		log.Printf("sync'd %d files of type: %s", count, name)

	}

	if err != nil {
		log.Println(err)
	}

}
