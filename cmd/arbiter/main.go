package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	configPath := flag.String("config", "config.json", "Arbiter local configuration file")
	isDaemon := flag.Bool("d", false, "Run shinsakuto in daemon mode")
	verifyOnly := flag.Bool("v", false, "Verify configuration syntax and exit")
	flag.Parse()

	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Critical: Failed to load config: %v", err)
	}

	if *verifyOnly {
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("CONFIG ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("SHINSAKUTO CONFIG VALID\n")
		fmt.Printf("- %d Hosts\n- %d Commands\n- %d HostGroups\n", 
			len(cfg.Hosts), len(cfg.Commands), len(cfg.HostGroups))
		os.Exit(0)
	}

	if *isDaemon && os.Getenv("ARBITER_DAEMON") != "true" {
		log.Println("Starting daemon...")
		daemonize(*configPath)
		return
	}

	log.Printf("shinsakuto Arbiter active (PID %d)", os.Getpid())
	go startWatcher()
	startAPI()
}
