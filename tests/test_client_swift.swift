import Foundation
import CakeAgentLib
import GRPC
import NIO
import SwiftProtobuf

struct CakeAgentSwiftClient {
    let channel: GRPCChannel
    let client: Cakeagent_CakeAgentServiceNIOClient
    
    init(host: String, port: Int) {
        let group = MultiThreadedEventLoopGroup(numberOfThreads: 1)
        
        self.channel = ClientConnection
            .insecure(group: group)
            .connect(host: host, port: port)
        
        self.client = Cakeagent_CakeAgentServiceNIOClient(channel: channel)
    }
    
    func getInfo() async throws {
        let request = Cakeagent_CakeAgent.Empty()
        
        do {
            let response = try await client.info(request).response.get()
            
            print("=== INFORMATIONS SYSTÈME ===")
            print("Version: \(response.version)")
            print("Nom d'hôte: \(response.hostname)")
            print("OS: \(response.osname)")
            print("Release: \(response.release)")
            print("Uptime: \(response.uptime)")
            print("Nombre de processeurs: \(response.cpuCount)")
            
            // Mémoire
            print("\n=== MÉMOIRE ===")
            print("Total: \(response.memory.total) bytes")
            print("Utilisée: \(response.memory.used) bytes")
            print("Libre: \(response.memory.free) bytes")
            
            // CPU
            if response.hasCpu {
                print("\n=== INFORMATIONS CPU ===")
                print("Utilisation totale: \(String(format: "%.2f", response.cpu.totalUsagePercent))%")
                print("User: \(String(format: "%.2f", response.cpu.user))%")
                print("System: \(String(format: "%.2f", response.cpu.system))%") 
                print("Idle: \(String(format: "%.2f", response.cpu.idle))%")
                
                print("\n=== INFORMATIONS PAR CŒUR ===")
                for (index, core) in response.cpu.cores.enumerated() {
                    print("Cœur \(core.coreID): \(String(format: "%.2f", core.usagePercent))% (User: \(String(format: "%.2f", core.user))%, System: \(String(format: "%.2f", core.system))%, Idle: \(String(format: "%.2f", core.idle))%)")
                }
            } else {
                print("\n=== CPU ===")
                print("Aucune information CPU détaillée disponible")
            }
            
            // Adresses IP
            print("\n=== ADRESSES IP ===")
            for ip in response.ipaddresses {
                print("IP: \(ip)")
            }
            
            // Disques
            print("\n=== INFORMATIONS DISQUE ===")
            for disk in response.diskInfos {
                print("Device: \(disk.device)")
                print("Mount: \(disk.mount)")
                print("FS Type: \(disk.fsType)")
                print("Taille: \(disk.size) bytes")
                print("Utilisé: \(disk.used) bytes")
                print("Libre: \(disk.free) bytes")
                print("---")
            }
            
        } catch {
            print("Erreur lors de la récupération des informations: \(error)")
        }
    }
    
    func close() {
        try? channel.close().wait()
    }
}

@main
struct Main {
    static func main() async {
        let client = CakeAgentSwiftClient(host: "localhost", port: 9999)
        
        print("Test du client Swift CakeAgent")
        print("Connexion au serveur sur localhost:9999...")
        
        await client.getInfo()
        
        client.close()
    }
}