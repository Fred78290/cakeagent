import Foundation
import ArgumentParser
import Logging
import CakeAgentLib

struct Infos: ParsableCommand {
	static let configuration: CommandConfiguration = CommandConfiguration(abstract: "Display MacOS system information")

	struct InfoReply: Codable {
		struct MemoryInfo: Codable {
			var total: UInt64 = 0
			var free: UInt64 = 0
			var used: UInt64 = 0

			enum CodingKeys: String, CodingKey {
				case total = "total"
				case free = "free"
				case used = "used"
			}

			init() {
				
			}

			init(from: CakeAgent.InfoReply.MemoryInfo) {
				self.total = from.total
				self.free = from.free
				self.used = from.used
			}
		}

		struct DiskInfo: Codable {
			var device: String = ""
			var mount: String = ""
			var fsType: String = ""
			var size: UInt64 = 0
			var used: UInt64 = 0
			var free: UInt64 = 0

			enum CodingKeys: String, CodingKey {
				case device = "device"
				case mount = "mount"
				case fsType = "fsType"
				case size = "size"
				case used = "used"
				case free = "free"
			}

			init() {
				
			}

			init(from: CakeAgent.InfoReply.DiskInfo) {
				self.device = from.device
				self.mount = from.mount
				self.fsType = from.fsType
				self.size = from.size
				self.used = from.used
				self.free = from.free
			}
		}

		struct CpuCoreInfo: Codable {
			var coreID: Int32 = 0
			var usagePercent: Double = 0
			var user: Double = 0
			var system: Double = 0
			var idle: Double = 0
			var iowait: Double = 0
			var irq: Double = 0
			var softirq: Double = 0
			var steal: Double = 0
			var guest: Double = 0
			var guestNice: Double = 0
			var nice: Double = 0

			enum CodingKeys: String, CodingKey {
				case coreID = "core_id"
				case usagePercent = "usage_percent"
				case user = "user"
				case system = "system"
				case idle = "idle"
				case iowait = "iowait"
				case irq = "irq"
				case softirq = "softirq"
				case steal = "steal"
				case guest = "guest"
				case guestNice = "guestNice"
				case nice = "nice"
			}

			init() {
				
			}

			init(from: Cakeagent_CakeAgent.InfoReply.CpuCoreInfo) {
				self.coreID = from.coreID
				self.usagePercent = from.usagePercent
				self.user = from.user
				self.system = from.system
				self.idle = from.idle
				self.iowait = from.iowait
				self.irq = from.irq
				self.softirq = from.softirq
				self.steal = from.steal
				self.guest = from.guest
				self.guestNice = from.guestNice
				self.nice = from.nice
			}
		}

		struct CpuInfo: Codable {
			var totalUsagePercent: Double = 0
			var user: Double = 0
			var system: Double = 0
			var idle: Double = 0
			var iowait: Double = 0
			var irq: Double = 0
			var softirq: Double = 0
			var steal: Double = 0
			var guest: Double = 0
			var guestNice: Double = 0
			var nice: Double = 0
			var cores: [CpuCoreInfo] = []

			enum CodingKeys: String, CodingKey {
				case totalUsagePercent = "total_usage_percent"
				case user = "user"
				case system = "system"
				case idle = "idle"
				case iowait = "iowait"
				case irq = "irq"
				case softirq = "softirq"
				case steal = "steal"
				case guest = "guest"
				case guestNice = "guestNice"
				case nice = "nice"
				case cores = "cores"
			}

			init() {
				
			}

			init(from: CakeAgent.InfoReply.CpuInfo) {
				self.totalUsagePercent = from.totalUsagePercent
				self.user = from.user
				self.system = from.system
				self.idle = from.idle
				self.iowait = from.iowait
				self.irq = from.irq
				self.softirq = from.softirq
				self.steal = from.steal
				self.guest = from.guest
				self.guestNice = from.guestNice
				self.nice = from.nice
				self.cores = from.cores.map { CpuCoreInfo(from: $0) }
			}
		}

		var version: String = ""
		var uptime: UInt64 = 0
		var cpuCount: Int32 = 0
		var memory: MemoryInfo = .init()
		var diskInfos: [DiskInfo] = .init()
		var ipaddresses: [String] = .init()
		var osname: String = ""
		var hostname: String = ""
		var release: String = ""
		var cpu: CpuInfo = .init()
		var agentVersion: String = ""

		enum CodingKeys: String, CodingKey {
			case version = "version"
			case uptime = "uptime"
			case cpuCount = "cpuCount"
			case memory = "memory"
			case diskInfos = "diskInfos"
			case ipaddresses = "ipaddresses"
			case osname = "osname"
			case hostname = "hostname"
			case release = "release"
			case cpu = "cpu"
			case agentVersion = "agentVersion"
		}

		init(from: CakeAgent.InfoReply) {
			self.version = from.version
			self.uptime = from.uptime
			self.cpuCount = from.cpuCount
			self.memory = MemoryInfo(from: from.memory)
			self.diskInfos = from.diskInfos.map { DiskInfo(from: $0) }
			self.ipaddresses = from.ipaddresses
			self.osname = from.osname
			self.hostname = from.hostname
			self.release = from.release
			self.cpu = CpuInfo(from: from.cpu)
			self.agentVersion = from.agentVersion
		}
	}

	@Option(name: [.customLong("log-level")], help: "Log level")
	var logLevel: Logging.Logger.Level = .info

	@Flag(help: "Output format: text or json")
	var format: Format = .text

	func validate() throws {
		Logger.setLevel(self.logLevel)
	}

	func run() throws {
		let result = InfoReply(from: InfosHandler.infos(cpuWatcher: .init()))

		print(self.format.renderSingle(result))
	}
}
