package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func main() {
	cfgPath := flag.String("config", "config.json", "Path to JSON config file")
	daemon := flag.Bool("d", false, "Run in background (daemon mode)")
	verifyOnly := flag.Bool("v", false, "Verify configuration and exit")
	flag.Parse()

	// 1. Load Local Node Settings
	if err := loadArbiterLocalConfig(*cfgPath); err != nil {
		log.Fatalf("Critical: Failed to load config: %v", err)
	}

	// 2. Verification Mode (-v)
	if *verifyOnly {
		fmt.Printf("Analyzing definitions in: %s\n", appConfig.DefinitionsDir)
		cfg, err := loadAndProcess()
		if err != nil {
			fmt.Printf("[ERR] YAML Error: %v\n", err)
			os.Exit(1)
		}

		audit := RunLinter(cfg)

		fmt.Println("\n-------------------------------------------")
		fmt.Println(" SHINSAKUTO ARBITER CONFIGURATION REPORT")
		fmt.Println("-------------------------------------------")
		fmt.Printf(" Hosts:          %d\n", audit.Counts.Hosts)
		fmt.Printf(" Services:       %d\n", audit.Counts.Services)
		fmt.Printf(" Commands:       %d\n", audit.Counts.Commands)
		fmt.Printf(" TimePeriods:    %d\n", audit.Counts.TimePeriods)
		fmt.Printf(" Contacts:       %d\n", audit.Counts.Contacts)
		fmt.Printf(" HostGroups:     %d\n", audit.Counts.HostGroups)
		fmt.Printf(" ServiceGroups:  %d\n", audit.Counts.ServiceGroups)
		fmt.Println("-------------------------------------------")

		for _, w := range audit.Warnings { fmt.Printf("[WARN] %s\n", w) }
		if len(audit.Errors) > 0 {
			for _, e := range audit.Errors { fmt.Printf("[ERR] %s\n", e) }
			fmt.Println("\nResult: INVALID CONFIGURATION")
			os.Exit(1)
		}
		fmt.Println("\nResult: VALID CONFIGURATION")
		os.Exit(0)
	}

	// 3. Daemon Mode (-d)
	if *daemon {
		args := []string{"-config", *cfgPath}
		cmd := exec.Command(os.Args[0], args...)
		if err := cmd.Start(); err != nil {
			log.Fatalf("Failed to start daemon: %v", err)
		}
		fmt.Printf("Arbiter started in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 4. HA Initialization
	if appConfig.HAEnabled {
		if err := setupRaft(); err != nil {
			log.Fatalf("Raft initialization failed: %v", err)
		}
	} else {
		log.Println("[INFO] High Availability is DISABLED. Running in Standalone mode.")
	}

	// 5. Lifecycle Management
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go startWatcher(ctx)
	go startAPI()

	log.Printf("[MAIN] Arbiter operational (Port %d). Press Ctrl+C to stop.", appConfig.APIPort)
	<-ctx.Done()
	log.Println("[MAIN] Shutting down Arbiter gracefully...")
}
