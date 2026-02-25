package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	configPath := flag.String("config", "config.json", "Config path")
	isDaemon := flag.Bool("d", false, "Daemon mode")
	verifyOnly := flag.Bool("v", false, "Verify syntax")
	flag.Parse()

	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Erreur config : %v", err)
	}

	if *verifyOnly {
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("ERREUR : %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Valide : %d hôtes, %d commandes.\n", len(cfg.Hosts), len(cfg.Commands))
		os.Exit(0)
	}

	if *isDaemon && os.Getenv("ARBITER_DAEMON") != "true" {
		daemonize(*configPath)
		return
	}

	log.Printf("Arbiter démarré (PID %d)", os.Getpid())
	go startWatcher()
	startAPI()
}
