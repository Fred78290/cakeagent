import Foundation

// Extension pour créer facilement les structures CPU avec des valeurs par défaut
extension Cakeagent_CakeAgent.InfoReply.CpuCoreInfo {
    public static func create(
        coreID: Int32,
        usagePercent: Double,
        user: Double = 0.0,
        system: Double = 0.0,
        idle: Double = 0.0,
        nice: Double = 0.0
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
        coreInfo.nice = nice
        return coreInfo
    }
}

extension Cakeagent_CakeAgent.InfoReply.CpuInfo {
    public static func create(
        totalUsagePercent: Double,
        cores: [Cakeagent_CakeAgent.InfoReply.CpuCoreInfo],
        user: Double = 0.0,
        system: Double = 0.0,
        idle: Double = 0.0,
        nice: Double = 0.0 
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
        cpuInfo.nice = nice
        return cpuInfo
    }
}

extension CPUWatcher {
    public func currentUsage() -> Cakeagent_CakeAgent.CurrentUsageReply {
        let (numOfCpus, cpuInfo) = self.collect()

        return Cakeagent_CakeAgent.CurrentUsageReply.with {
            $0.cpuCount = numOfCpus
            $0.cpuInfos = cpuInfo
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

	public func collect() -> (numOfCpus: Int32, cpuInfo: Cakeagent_CakeAgent.InfoReply.CpuInfo) {
		// Créer les informations par cœur
		let cpuLoad = self.refresh()
		var cores: [CakeAgent.InfoReply.CpuCoreInfo] = []

		for coreInfo in cpuLoad.usagePerCore {
			let core = CakeAgent.InfoReply.CpuCoreInfo.create(
				coreID: coreInfo.coreID,
				usagePercent: coreInfo.usage,
				user: Double(coreInfo.user),
				system: Double(coreInfo.system),
				idle: Double(coreInfo.idle),
				nice: Double(coreInfo.nice)
			)
			cores.append(core)
		}
		
		// Créer l'information CPU globale
		let cpuInfo = CakeAgent.InfoReply.CpuInfo.create(
			totalUsagePercent: cpuLoad.totalUsage,
			cores: cores,
			user: cpuLoad.userLoad,
			system: cpuLoad.systemLoad,
			idle: cpuLoad.idleLoad,
			nice: cpuLoad.niceLoad
		)

		return (Int32(ProcessInfo.processInfo.processorCount), cpuInfo)
	}
}
