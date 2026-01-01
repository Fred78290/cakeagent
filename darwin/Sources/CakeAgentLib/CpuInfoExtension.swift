import Foundation

// Extension pour créer facilement les structures CPU avec des valeurs par défaut
extension Cakeagent_CakeAgent.InfoReply.CpuCoreInfo {
    public static func create(
        coreID: Int32,
        usagePercent: Double,
        user: Double = 0.0,
        system: Double = 0.0,
        idle: Double = 0.0
    ) -> Cakeagent_CakeAgent.InfoReply.CpuCoreInfo {
        var coreInfo = Cakeagent_CakeAgent.InfoReply.CpuCoreInfo()
        coreInfo.coreID = coreID
        coreInfo.usagePercent = usagePercent
        coreInfo.user = user
        coreInfo.system = system
        coreInfo.idle = idle
        coreInfo.iowait = 0.0
        coreInfo.irq = 0.0
        coreInfo.softirq = 0.0
        coreInfo.steal = 0.0
        coreInfo.guest = 0.0
        coreInfo.guestNice = 0.0
        return coreInfo
    }
}

extension Cakeagent_CakeAgent.CurrentUsageReply {
    public static func collect() -> Cakeagent_CakeAgent.CurrentUsageReply {
        let cpuInfos = Cakeagent_CakeAgent.InfoReply.CpuInfo.collect()

        return Cakeagent_CakeAgent.CurrentUsageReply.with {
            $0.cpuCount = Int32(cpuInfos.numOfCpus)
            $0.cpuInfos = cpuInfos.cpuInfo
			$0.memory = .with {
                var size: size_t = 0
                var memSize: UInt64 = 0
                var freeMemory: UInt64 = 0

                size = MemoryLayout<UInt64>.size
                sysctlbyname("hw.memsize", &memSize, &size, nil, 0)
                
                size = MemoryLayout<UInt64>.size
                sysctlbyname("vm.page_free_count", &freeMemory, &size, nil, 0)

                $0.free = freeMemory * UInt64(vm_page_size)
                $0.total = ProcessInfo.processInfo.physicalMemory
                $0.used = $0.total - $0.free
            }
        }
    }
}

extension Cakeagent_CakeAgent.InfoReply.CpuInfo {
    public static func create(
        totalUsagePercent: Double,
        cores: [Cakeagent_CakeAgent.InfoReply.CpuCoreInfo],
        user: Double = 0.0,
        system: Double = 0.0,
        idle: Double = 0.0
    ) -> Cakeagent_CakeAgent.InfoReply.CpuInfo {
        var cpuInfo = Cakeagent_CakeAgent.InfoReply.CpuInfo()
        cpuInfo.totalUsagePercent = totalUsagePercent
        cpuInfo.cores = cores
        cpuInfo.user = user
        cpuInfo.system = system
        cpuInfo.idle = idle
        cpuInfo.iowait = 0.0
        cpuInfo.irq = 0.0
        cpuInfo.softirq = 0.0
        cpuInfo.steal = 0.0
        cpuInfo.guest = 0.0
        cpuInfo.guestNice = 0.0
        return cpuInfo
    }

    public static func collect() -> (numOfCpus: Int, cpuInfo: Cakeagent_CakeAgent.InfoReply.CpuInfo) {
        // Collecter les informations CPU
        let coreInfos = MacOSCPUCollector.getCPUInfo()
        let globalCPUInfo = MacOSCPUCollector.getGlobalCPUInfo()
        
        // Créer les informations par cœur
        var cores: [CakeAgent.InfoReply.CpuCoreInfo] = []
        for coreInfo in coreInfos {
            let core = CakeAgent.InfoReply.CpuCoreInfo.create(
                coreID: coreInfo.coreID,
                usagePercent: coreInfo.usage,
                user: coreInfo.user,
                system: coreInfo.system,
                idle: coreInfo.idle
            )
            cores.append(core)
        }
        
        // Créer l'information CPU globale
        let cpuInfo = CakeAgent.InfoReply.CpuInfo.create(
            totalUsagePercent: globalCPUInfo.totalUsage,
            cores: cores,
            user: globalCPUInfo.user,
            system: globalCPUInfo.system,
            idle: globalCPUInfo.idle
        )

        return (ProcessInfo.processInfo.processorCount, cpuInfo)
    }
}