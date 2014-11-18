package main

import (
	"log"

	"github.com/jason0x43/go-alfred"
)

func main() {
	err := alfred.ShowMessage("Something", "Click me")
	if err != nil {
		log.Printf("Error showing message: %v", err)
	}
	log.Printf("Showed message")
}
