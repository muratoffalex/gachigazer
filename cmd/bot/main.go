package main

import (
	"fmt"
	"log"

	"github.com/muratoffalex/gachigazer/internal/app"
)

var (
	version   string
	buildTime string
)

func main() {
	fmt.Printf("Starting application version: %s (built at: %s)\n", version, buildTime)
	application, err := app.New()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	if err := application.Start(); err != nil {
		application.Logger.WithError(err).Fatal("Application failed")
	}

	application.WaitForShutdown()
}
