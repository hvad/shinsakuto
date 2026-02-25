package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	configPath := flag.String("config", "config.json", "Arbiter configuration file path")
	isDaemon := flag.Bool("d", false, "Run in background (daemon mode)")
	verifyOnly := flag.Bool("v", false, "Verify syntax only and exit")
	flag.Parse()

	// Load local settings (Scheduler URL, API Port, etc.)
	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Could not load local config: %v", err)
	}

	if *verifyOnly {
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Config valid:\n- %d Hosts\n- %d Commands\n- %d Contacts\n", 
			len(cfg.Hosts), len(cfg.Commands), len(cfg.Contacts))
		os.Exit(0)
	}

	// Handle background execution
	if *isDaemon && os.Getenv("ARBITER_DAEMON") != "true" {
		log.Println("Switching to daemon mode...")
		daemonize(*configPath)
		return
	}

	log.Printf("Arbiter Go active (PID %d)", os.Getpid())
	go startWatcher() 
	startAPI()        
}
