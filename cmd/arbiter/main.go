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
	cfgPath := flag.String("config", "config.json", "Path to JSON configuration file")
	daemon := flag.Bool("d", false, "Run Arbiter in background (daemon mode)")
	verifyOnly := flag.Bool("v", false, "Verify YAML definitions and exit")
	flag.Parse()

	if err := loadArbiterLocalConfig(*cfgPath); err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}

	// 3. Verification Mode (-v)
	if *verifyOnly {
		fmt.Printf("Analyzing definitions in: %s\n", appConfig.DefinitionsDir)
		cfg, err := loadAndProcess()
		if err != nil {
			fmt.Printf("[ERROR] YAML Processing failed: %v\n", err)
			os.Exit(1)
		}

		audit := RunLinter(cfg)

		fmt.Println("\n-------------------------------------------")
		fmt.Println(" SHINSAKUTO CONFIGURATION REPORT")
		fmt.Println("-------------------------------------------")
		fmt.Printf(" Active Hosts:          %d\n", audit.Counts.Hosts)
		fmt.Printf(" Active Services:       %d\n", audit.Counts.Services)
		fmt.Printf(" Commands:              %d\n", audit.Counts.Commands)
		fmt.Printf(" TimePeriods:           %d\n", audit.Counts.TimePeriods)
		fmt.Printf(" Contacts:              %d\n", audit.Counts.Contacts)
		fmt.Printf(" Host Groups:           %d\n", audit.Counts.HostGroups)
		fmt.Printf(" Service Groups:        %d\n", audit.Counts.ServiceGroups)
		fmt.Println("-------------------------------------------")

		for _, w := range audit.Warnings {
			fmt.Printf("[WARNING] %s\n", w)
		}

		if len(audit.Errors) > 0 {
			for _, e := range audit.Errors {
				fmt.Printf("[ERROR] %s\n", e)
			}
			fmt.Println("\nResult: INVALID CONFIGURATION")
			os.Exit(1)
		}

		fmt.Println("\nResult: CONFIGURATION VALID")
		os.Exit(0)
	}

	// 4. Daemon Mode (-d)
	if *daemon {
		cmd := exec.Command(os.Args[0], "-config", *cfgPath)
		if err := cmd.Start(); err != nil {
			log.Fatalf("[ERROR] Failed to start daemon: %v", err)
		}
		fmt.Printf("Arbiter started in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 5. HA Initialization
	if appConfig.HAEnabled {
		log.Printf("[HA] Initializing Raft node: %s", appConfig.RaftNodeID)
		if err := setupRaft(); err != nil {
			log.Fatalf("[FATAL] Raft error: %v", err)
		}
	} else {
		log.Println("[INFO] Solo Mode (High Availability disabled)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go startWatcher(ctx)
	go startAPI()

	log.Printf("[MAIN] Arbiter operational on port %d", appConfig.APIPort)
	
	<-ctx.Done()
	log.Println("[MAIN] Shutting down Arbiter...")
}
