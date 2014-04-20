package main

import (
	"github.com/jason0x43/go-log"
	"github.com/jason0x43/go-alfred"
)

func main() {
	log.Level = log.LEVEL_WARN
	workflow, err := alfred.OpenWorkflow(".")
	if err != nil {
		log.Fatal("error opening workflow: %s", err)
	}

	log.Printf("cache dir: %s\n", workflow.CacheDir())
}
