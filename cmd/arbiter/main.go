package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to shinsakuto config file")
	isDaemon := flag.Bool("d", false, "Run in daemon mode")
	verifyOnly := flag.Bool("v", false, "Verify configuration and exit")
	flag.Parse()

	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Critical error: %v", err)
	}

	if *verifyOnly {
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("CONFIG ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("SHINSAKUTO CONFIG VALID\n")
		fmt.Printf("- %d Hosts\n- %d Services\n- %d Groups\n", 
			len(cfg.Hosts), len(cfg.Services), len(cfg.HostGroups))
		os.Exit(0)
	}

	if *isDaemon && os.Getenv("ARBITER_DAEMON") != "true" {
		log.Println("Background mode enabled. Starting daemon...")
		daemonize(*configPath)
		return
	}

	log.Printf("shinsakuto Arbiter starting (PID %d)", os.Getpid())
	go startWatcher()
	startAPI()
}
