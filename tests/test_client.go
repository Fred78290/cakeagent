package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"cakeagent"
)

func main() {
	// Connexion au serveur Swift sur port 9999
	conn, err := grpc.Dial("localhost:9999", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Échec de connexion: %v", err)
	}
	defer conn.Close()

	client := cakeagent.NewCakeAgentServiceClient(conn)

	// Appel de la méthode Info
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	resp, err := client.Info(ctx, &cakeagent.CakeAgent_Empty{})
	if err != nil {
		log.Fatalf("Échec de l'appel Info: %v", err)
	}

	// Affichage des informations générales
	fmt.Printf("=== Informations Système ===\n")
	fmt.Printf("Hostname: %s\n", resp.Hostname)
	fmt.Printf("OS: %s\n", resp.Osname)
	fmt.Printf("Version: %s\n", resp.Version)
	fmt.Printf("CPU Count: %d\n", resp.CpuCount)
	fmt.Printf("Uptime: %d seconds\n", resp.Uptime)

	// Affichage des informations CPU détaillées
	if resp.Cpu != nil {
		fmt.Printf("\n=== Informations CPU ===\n")
		fmt.Printf("Utilisation totale: %.2f%%\n", resp.Cpu.TotalUsagePercent)
		fmt.Printf("User: %.2f%%\n", resp.Cpu.User)
		fmt.Printf("System: %.2f%%\n", resp.Cpu.System)
		fmt.Printf("Idle: %.2f%%\n", resp.Cpu.Idle)
		fmt.Printf("IOWait: %.2f%%\n", resp.Cpu.Iowait)

		// Affichage des informations par cœur
		if len(resp.Cpu.Cores) > 0 {
			fmt.Printf("\n=== Informations par Cœur ===\n")
			for _, core := range resp.Cpu.Cores {
				fmt.Printf("Cœur %d: %.2f%% (User: %.2f%%, System: %.2f%%, Idle: %.2f%%, IOWait: %.2f%%)\n",
					core.CoreId, core.UsagePercent, core.User, core.System, core.Idle, core.Iowait)
			}
		}
	}

	// Affichage des informations mémoire
	if resp.Memory != nil {
		fmt.Printf("\n=== Informations Mémoire ===\n")
		fmt.Printf("Total: %d MB\n", resp.Memory.Total/(1024*1024))
		fmt.Printf("Utilisée: %d MB\n", resp.Memory.Used/(1024*1024))
		fmt.Printf("Libre: %d MB\n", resp.Memory.Free/(1024*1024))
	}

	// Affichage au format JSON pour debug
	jsonData, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		log.Printf("Erreur lors de la conversion JSON: %v", err)
	} else {
		fmt.Printf("\n=== Réponse complète (JSON) ===\n%s\n", string(jsonData))
	}
}
