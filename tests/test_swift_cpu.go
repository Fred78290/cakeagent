package main

import (
	"context"
	"log"
	"time"

	pb "github.com/fboltz/cakeagent/cakeagent"
	"google.golang.org/grpc"
)

func main() {
	// Connexion au serveur Swift
	conn, err := grpc.Dial("localhost:9999", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Impossible de se connecter au serveur: %v", err)
	}
	defer conn.Close()

	client := pb.NewCakeAgentServiceClient(conn)

	// Créer un contexte avec timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Appeler la méthode Info
	req := &pb.Empty{}
	resp, err := client.Info(ctx, req)
	if err != nil {
		log.Fatalf("Échec de l'appel Info: %v", err)
	}

	// Afficher les informations de base
	log.Printf("=== INFORMATIONS SYSTÈME SWIFT ===")
	log.Printf("Hostname: %s", resp.Hostname)
	log.Printf("OS: %s", resp.Osname)
	log.Printf("Release: %s", resp.Release)
	log.Printf("Nombre de processeurs: %d", resp.CpuCount)

	// Vérifier les informations CPU
	if resp.Cpu != nil {
		log.Printf("\n=== INFORMATIONS CPU ===")
		log.Printf("Utilisation totale: %.2f%%", resp.Cpu.TotalUsagePercent)
		log.Printf("User: %.2f%%", resp.Cpu.User)
		log.Printf("System: %.2f%%", resp.Cpu.System)
		log.Printf("Idle: %.2f%%", resp.Cpu.Idle)

		log.Printf("\n=== INFORMATIONS PAR CŒUR ===")
		for i, core := range resp.Cpu.Cores {
			log.Printf("Cœur %d (ID %d): %.2f%% (User: %.2f%%, System: %.2f%%, Idle: %.2f%%)",
				i, core.CoreId, core.UsagePercent, core.User, core.System, core.Idle)
		}
	} else {
		log.Printf("Aucune information CPU détaillée disponible")
	}

	// Informations mémoire
	if resp.Memory != nil {
		log.Printf("\n=== MÉMOIRE ===")
		log.Printf("Total: %d bytes", resp.Memory.Total)
		log.Printf("Utilisée: %d bytes", resp.Memory.Used)
		log.Printf("Libre: %d bytes", resp.Memory.Free)
	}

	log.Printf("Test terminé avec succès !")
}
