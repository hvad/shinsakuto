package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	configPath := flag.String("config", "config.json", "Internal configuration file path")
	verifyOnly := flag.Bool("v", false, "Verify configuration integrity and exit")
	flag.Parse()

	// Load base arbiter settings
	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Fatal: could not load config file: %v", err)
	}

	// Logic for -v option
	if *verifyOnly {
		fmt.Println("Shinsakuto: Starting deep verification...")
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("CONFIG ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("CONFIG VALID\n")
		fmt.Printf("  -> Hosts: %d\n  -> Services: %d\n  -> Commands: %d\n  -> Contacts: %d\n  -> Host goups: %d\n  -> Service goups: %d\n", 
			len(cfg.Hosts), len(cfg.Services), len(cfg.Commands), len(cfg.Contacts), len(cfg.HostGroups), len(cfg.ServiceGroups))
		os.Exit(0)
	}

	log.Printf("shinsakuto Arbiter active (PID %d)", os.Getpid())
	go startWatcher()
	startAPI()
}
