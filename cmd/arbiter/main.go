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
	// 1. Définition des arguments de la ligne de commande
	cfgPath := flag.String("config", "config.json", "Chemin vers le fichier de configuration JSON")
	daemon := flag.Bool("d", false, "Lancer l'Arbiter en arrière-plan (mode daemon)")
	verifyOnly := flag.Bool("v", false, "Vérifier la validité des définitions YAML et quitter")
	flag.Parse()

	// 2. Chargement de la configuration locale du nœud
	if err := loadArbiterLocalConfig(*cfgPath); err != nil {
		log.Fatalf("[FATAL] Impossible de charger la configuration : %v", err)
	}

	// 3. Mode Vérification (-v)
	// Utile pour valider la syntaxe et l'héritage sans démarrer le service
	if *verifyOnly {
		fmt.Printf("Analyse des définitions dans : %s\n", appConfig.DefinitionsDir)
		cfg, err := loadAndProcess()
		if err != nil {
			fmt.Printf("[ERREUR] Échec du traitement YAML : %v\n", err)
			os.Exit(1)
		}

		audit := RunLinter(cfg)

		fmt.Println("\n-------------------------------------------")
		fmt.Println(" RAPPORT DE CONFIGURATION SHINSAKUTO")
		fmt.Println("-------------------------------------------")
		fmt.Printf(" Hôtes actifs :          %d\n", audit.Counts.Hosts)
		fmt.Printf(" Services actifs :       %d\n", audit.Counts.Services)
		fmt.Printf(" Commandes :             %d\n", audit.Counts.Commands)
		fmt.Printf(" Périodes (TimePeriods): %d\n", audit.Counts.TimePeriods)
		fmt.Printf(" Contacts :              %d\n", audit.Counts.Contacts)
		fmt.Printf(" Groupes d'hôtes :       %d\n", audit.Counts.HostGroups)
		fmt.Printf(" Groupes de services :   %d\n", audit.Counts.ServiceGroups)
		fmt.Println("-------------------------------------------")

		// Affichage des avertissements (Warnings)
		for _, w := range audit.Warnings {
			fmt.Printf("[ATTENTION] %s\n", w)
		}

		// Gestion des erreurs critiques
		if len(audit.Errors) > 0 {
			for _, e := range audit.Errors {
				fmt.Printf("[ERREUR] %s\n", e)
			}
			fmt.Println("\nRésultat : CONFIGURATION INVALIDE")
			os.Exit(1)
		}

		fmt.Println("\nRésultat : CONFIGURATION VALIDE")
		os.Exit(0)
	}

	// 4. Mode Daemon (-d)
	// Relance le binaire en arrière-plan
	if *daemon {
		args := []string{"-config", *cfgPath}
		cmd := exec.Command(os.Args[0], args...)
		if err := cmd.Start(); err != nil {
			log.Fatalf("[ERREUR] Impossible de lancer le daemon : %v", err)
		}
		fmt.Printf("Arbiter démarré en arrière-plan (PID: %d)\n", cmd.Process.Pid)
		os.Exit(0)
	}

	// 5. Initialisation de la Haute Disponibilité (HA)
	if appConfig.HAEnabled {
		log.Printf("[HA] Initialisation du nœud Raft : %s", appConfig.RaftNodeID)
		if err := setupRaft(); err != nil {
			log.Fatalf("[FATAL] Erreur Raft : %v", err)
		}
	} else {
		log.Println("[INFO] Mode Solo activé (Haute Disponibilité désactivée)")
	}

	// 6. Gestion du cycle de vie et signaux système
	// Permet un arrêt propre avec Ctrl+C ou SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 7. Lancement des services core
	go startWatcher(ctx) // Surveillance des fichiers et sync Scheduler
	go startAPI()        // API HTTP pour l'administration et le cluster

	log.Printf("[MAIN] Arbiter opérationnel sur le port %d", appConfig.APIPort)
	
	// Attend le signal d'arrêt
	<-ctx.Done()
	
	log.Println("[MAIN] Arrêt de l'Arbiter en cours...")
	// Note : Raft et les serveurs HTTP se ferment proprement via le contexte ou le processus
}
