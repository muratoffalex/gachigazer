package main

import (
	"log"

	"github.com/muratoffalex/gachigazer/internal/app"
)

func main() {
	application, err := app.New()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	if err := application.Start(); err != nil {
		application.Logger.WithError(err).Fatal("Application failed")
	}

	application.WaitForShutdown()
}
