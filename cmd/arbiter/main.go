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
	// CLI Arguments parsing
	configPath := flag.String("config", "config.json", "Internal configuration file path")
	verifyOnly := flag.Bool("v", false, "Verify configuration and exit")
	daemonMode := flag.Bool("d", false, "Run as a daemon in background")
	flag.Parse()

	// 1. Handle Daemon Mode
	if *daemonMode {
		args := os.Args[1:]
		cleanArgs := make([]string, 0)
		for _, arg := range args {
			if arg != "-d" { cleanArgs = append(cleanArgs, arg) }
		}
		cmd := exec.Command(os.Args[0], cleanArgs...)
		if err := cmd.Start(); err != nil {
			fmt.Printf("Failed to daemonize: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Arbiter started in background (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 2. Load settings and init dual-logging
	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Critical init failure: %v", err)
	}

	// 3. Configuration Verification Mode (-v)
	if *verifyOnly {
		fmt.Println("Shinsakuto Arbiter: Starting deep verification...")
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("FAILED: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("SUCCESS: Config is healthy (%d Hosts, %d Services, %d Commands, %d Contacts)\n", 
			len(cfg.Hosts), len(cfg.Services), len(cfg.Commands), len(cfg.Contacts))
		os.Exit(0)
	}

	// 4. Graceful Shutdown Signal Trapping
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Printf("shinsakuto Arbiter operational (PID: %d)", os.Getpid())

	// Start concurrent components
	go startWatcher(ctx)
	go startAPI()

	// Wait for termination signal
	<-ctx.Done()
	log.Println("[Main] Signal received, stopping services...")
	stopAPI()
	log.Println("[Main] Arbiter stopped clean.")
}
