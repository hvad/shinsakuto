package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"shinsakuto/pkg/logger"
)

func main() {
	cfgPath := flag.String("c", "config.json", "Path to JSON configuration file")
	daemon := flag.Bool("d", false, "Run Arbiter in background")
	verifyOnly := flag.Bool("v", false, "Verify YAML definitions and exit")
	flag.Parse()

	if err := loadArbiterLocalConfig(*cfgPath); err != nil {
		// Using standard log here because logger.Setup hasn't run yet if load fails early
		fmt.Fprintf(os.Stderr, "[FATAL] Arbiter : Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 3. Verification Mode (-v)
	if *verifyOnly {
		logger.Always("Analyzing definitions in: %s", appConfig.DefinitionsDir)
		cfg, err := loadAndProcess()
		if err != nil {
			logger.Always("[ERROR] YAML Processing failed: %v", err)
			os.Exit(1)
		}

		audit := RunLinter(cfg)

		fmt.Println("\n-------------------------------------------")
		fmt.Println(" Shinsakuto Configuration Report")
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
			logger.Always("[WARNING] %s", w)
		}

		if len(audit.Errors) > 0 {
			for _, e := range audit.Errors {
				logger.Always("[ERROR] %s", e)
			}
			logger.Always("Result: Invalid Configuration")
			os.Exit(1)
		}

		logger.Always("Result: Configuration Valid")
		os.Exit(0)
	}

	// 4. Daemon Mode (-d)
	if *daemon {
		cmd := exec.Command(os.Args[0], "-config", *cfgPath)
		if err := cmd.Start(); err != nil {
			logger.Fatal("Arbiter failed to start daemon: %v", err)
		}
		logger.Always("Arbiter started in background (PID: %d)", cmd.Process.Pid)
		os.Exit(0)
	}

	// 5. HA Initialization
	if appConfig.HAEnabled {
		logger.Always("[HA] Initializing Raft node: %s", appConfig.RaftNodeID)
		if err := setupRaft(); err != nil {
			logger.Fatal("Raft error: %v", err)
		}
	} else {
		logger.Always("Arbiter Standalone mode")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go startWatcher(ctx)
	go startAPI()

	logger.Always("[START] Arbiter listening on port %d", appConfig.Port)
	
	<-ctx.Done()
	logger.Always("[STOP] Shutting down Arbiter...")
}
