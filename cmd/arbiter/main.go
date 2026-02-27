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
	verifyOnly := flag.Bool("v", false, "Verify configuration integrity and exit")
	daemonMode := flag.Bool("d", false, "Run the process in background")
	flag.Parse()

	// 1. Daemonization
	if *daemonMode {
		args := os.Args[1:]
		var cleanArgs []string
		for _, a := range args { if a != "-d" { cleanArgs = append(cleanArgs, a) } }
		cmd := exec.Command(os.Args[0], cleanArgs...)
		if err := cmd.Start(); err != nil {
			log.Fatalf("Failed to daemonize: %v", err)
		}
		fmt.Printf("Arbiter daemonized (PID %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 2. Initial Config Load
	if err := loadArbiterLocalConfig(*configPath); err != nil {
		log.Fatalf("Critical error during config load: %v", err)
	}

	// 3. Verification Mode (-v)
	if *verifyOnly {
		fmt.Println("Shinsakuto Arbiter: Checking active definitions...")
		cfg, err := loadAndValidateAll()
		if err != nil {
			fmt.Printf("FAILURE: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("SUCCESS: Config valid (%d active hosts, %d active services)\n", len(cfg.Hosts), len(cfg.Services))
		os.Exit(0)
	}

	// 4. Runtime logic with Graceful Stop
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("shinsakuto Arbiter operational (PID %d)", os.Getpid())

	go startWatcher(ctx)
	go startAPI()

	<-ctx.Done()
	stopAPI() // Shutdown the HTTP server cleanly
	log.Println("Arbiter stopped.")
}
