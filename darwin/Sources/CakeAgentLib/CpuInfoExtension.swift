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
}