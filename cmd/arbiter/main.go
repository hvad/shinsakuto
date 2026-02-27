package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
)

func main() {
	// Define CLI flags
	configPath := flag.String("config", "config.json", "Internal configuration file path")
	verifyOnly := flag.Bool("v", false, "Verify configuration and exit")
	daemonMode := flag.Bool("d", false, "Run Arbiter as a background daemon")
	flag.Parse()

	// 1. Handle Daemon Mode: restart the process without -d in the background
	if *daemonMode {
		args := os.Args[1:]
		cleanArgs := make([]string, 0)
		for _, arg := range args {
			if arg != "-d" { cleanArgs = append(cleanArgs, arg) }
		}
		
		cmd := exec.Command(os.Args[0], cleanArgs...)
		if err := cmd.Start(); err != nil {
			fmt.Printf("Failed to start daemon: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Shinsakuto: Arbiter daemon started (PID %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 2. Load Local Config (Settings like API Port, Log file)
	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Fatal initialization error: %v", err)
	}

	// 3. Handle Verification Mode: Parse files and exit with report
	if *verifyOnly {
		fmt.Println("Shinsakuto: Starting deep verification...")
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("CONFIG ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("CONFIG VALID")
		fmt.Printf("  -> Hosts: %d\n  -> Services: %d\n  -> Commands: %d\n  -> Contacts: %d\n  -> TimePeriods: %d\n  -> HostGroups: %d\n  -> ServiceGroups: %d\n", 
			len(cfg.Hosts), len(cfg.Services), len(cfg.Commands), len(cfg.Contacts), len(cfg.TimePeriods), len(cfg.HostGroups), len(cfg.ServiceGroups))
		os.Exit(0)
	}

	// 4. Normal Daemon Operation
	log.Printf("shinsakuto Arbiter starting (PID %d)", os.Getpid())
	
	// Start the configuration file watcher in a goroutine
	go startWatcher()
	
	// Start the API server (blocking call)
	startAPI()
}
