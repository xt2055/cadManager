package main

import (
	"log"

	"cadguanliq/internal/app"
	"cadguanliq/internal/config"
)

func main() {
	if err := app.Run(config.Load()); err != nil {
		log.Fatal(err)
	}
}
