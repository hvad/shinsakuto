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
	configPath := flag.String("config", "config.json", "Internal configuration file path")
	verifyOnly := flag.Bool("v", false, "Verify configuration and exit")
	daemonMode := flag.Bool("d", false, "Run as a daemon in background")
	flag.Parse()

	// 1. Daemonization logic
	if *daemonMode {
		args := os.Args[1:]
		cleanArgs := make([]string, 0)
		for _, arg := range args {
			if arg != "-d" { cleanArgs = append(cleanArgs, arg) }
		}
		cmd := exec.Command(os.Args[0], cleanArgs...)
		if err := cmd.Start(); err != nil {
			fmt.Printf("Daemon failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Arbiter daemon started (PID %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 2. Load settings
	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}

	// 3. Verification mode (-v)
	if *verifyOnly {
		fmt.Println("Shinsakuto: Verifying configuration...")
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("SUCCESS: Config valid (Hosts: %d, Services: %d)\n", len(cfg.Hosts), len(cfg.Services))
		os.Exit(0)
	}

	// 4. Signal trapping for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Printf("shinsakuto Arbiter starting (PID %d)", os.Getpid())

	go startWatcher(ctx)
	go startAPI()

	// Wait for OS signal
	<-ctx.Done()
	log.Println("[Main] Signal received, stopping arbiter...")

	stopAPI()
	log.Println("[Main] Arbiter stopped clean.")
}
