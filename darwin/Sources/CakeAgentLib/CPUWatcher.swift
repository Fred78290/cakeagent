import Foundation
import Darwin
import Cocoa

public class CoreInfo: Codable {
	public enum CoreType: Int, Codable {
		case unknown = -1
		case efficiency = 1
		case performance = 2
	}

	public var id: Int32
	public var type: CoreType
	
	public init(id: Int32, type: CoreType) {
		self.id = id
		self.type = type
	}
}

@Observable
public class CoreLoad {	
	public var usage: Double
	public var coreID: Int32
	public var user: Int32
	public var system: Int32
	public var idle: Int32
	public var nice: Int32
	
	public init(coreID: Int32, user: Int32 = 0, system: Int32 = 0, idle: Int32 = 0, nice: Int32 = 0, usage: Double = 0) {
		self.coreID = coreID
		self.user = user
		self.system = system
		self.idle = idle
		self.nice = nice
		self.usage = usage
	}
}

@Observable
public class CPULoad {
	private enum CodingKeys: String, CodingKey {
		case totalUsage
		case usagePerCore
		case usageECores
		case usagePCores
		case systemLoad
		case userLoad
		case idleLoad
	}
	
	public var totalUsage: Double
	public var usagePerCore: [CoreLoad]
	public var usageECores: Double?
	public var usagePCores: Double?
	public var systemLoad: Double
	public var userLoad: Double
	public var idleLoad: Double
	
	public init(totalUsage: Double = 0, usagePerCore: [CoreLoad] = [], usageECores: Double? = nil, usagePCores: Double? = nil, systemLoad: Double = 0, userLoad: Double = 0, idleLoad: Double = 0) {
		self.totalUsage = totalUsage
		self.usagePerCore = usagePerCore
		self.usageECores = usageECores
		self.usagePCores = usagePCores
		self.systemLoad = systemLoad
		self.userLoad = userLoad
		self.idleLoad = idleLoad
	}
}

@Observable
public class CpuInformations {
	public var name: String?
	public var physicalCores: Int8?
	public var logicalCores: Int8?
	public var eCores: Int32?
	public var pCores: Int32?
	public var cores: [CoreInfo]?
	public var eCoreFrequencies: [Int32]?
	public var pCoreFrequencies: [Int32]?
	
	public init(name: String? = nil, physicalCores: Int8? = nil, logicalCores: Int8? = nil, eCores: Int32? = nil, pCores: Int32? = nil, cores: [CoreInfo]? = nil, eCoreFrequencies: [Int32]? = nil, pCoreFrequencies: [Int32]? = nil) {
		self.name = name
		self.physicalCores = physicalCores
		self.logicalCores = logicalCores
		self.eCores = eCores
		self.pCores = pCores
		self.cores = cores
		self.eCoreFrequencies = eCoreFrequencies
		self.pCoreFrequencies = pCoreFrequencies
	}
}

public func sysctlByName(_ name: String) -> String {
	var sizeOfName = 0

	guard sysctlbyname(name, nil, &sizeOfName, nil, 0) == 0  else {
		print(POSIXError.Code(rawValue: errno).map { POSIXError($0) } ?? CocoaError(.fileReadUnknown))
		return ""
	}
	
	var nameChars = [CChar](repeating: 0, count: sizeOfName)

	guard sysctlbyname(name, &nameChars, &sizeOfName, nil, 0) == 0 else {
		print(POSIXError.Code(rawValue: errno).map { POSIXError($0) } ?? CocoaError(.fileReadUnknown))
		return ""
	}

	return String(cString: nameChars)
}

public func sysctlByName(_ name: String) -> Int64 {
	var num: Int64 = 0
	var size = MemoryLayout<Int64>.size
	
	if sysctlbyname(name, &num, &size, nil, 0) != 0 {
		print(POSIXError.Code(rawValue: errno).map { POSIXError($0) } ?? CocoaError(.fileReadUnknown))
	}
	
	return num
}

extension String {
	public func condenseWhitespace() -> String {
		let components = self.components(separatedBy: .whitespacesAndNewlines)
		return components.filter { !$0.isEmpty }.joined(separator: " ")
	}
	
	public var trimmed: String {
		var buf = [UInt8]()
		var trimming = true
		for c in self.utf8 {
			if trimming && c < 33 { continue }
			trimming = false
			buf.append(c)
		}
		
		while let last = buf.last, last < 33 {
			buf.removeLast()
		}
		
		buf.append(0)

		return String(cString: buf)
	}
	
	public func matches(_ regex: String) -> Bool {
		return self.range(of: regex, options: .regularExpression, range: nil, locale: nil) != nil
	}
}

/// Collecteur d'informations CPU pour macOS
@Observable
public class CPUWatcher {
	private var cpuInfo: processor_info_array_t!
	private var prevCpuInfo: processor_info_array_t?
	private var numCpuInfo: mach_msg_type_number_t = 0
	private var numPrevCpuInfo: mach_msg_type_number_t = 0
	private var numCPUs: uint = 0
	private let CPUUsageLock: NSLock = NSLock()
	private var previousInfo = host_cpu_load_info()
	private var hasHyperthreadingCores = false

	private var numCPUsU: natural_t = 0
	private var usagePerCore: [CoreLoad] = []
	private var cores: [CoreInfo]? = nil
	
	public var cpuLoad = CPULoad()

	private static func getIOProperties(_ entry: io_registry_entry_t) -> NSDictionary? {
		var properties: Unmanaged<CFMutableDictionary>? = nil
		
		if IORegistryEntryCreateCFProperties(entry, &properties, kCFAllocatorDefault, 0) != kIOReturnSuccess {
			return nil
		}
		
		defer {
			properties?.release()
		}
		
		return properties?.takeUnretainedValue()
	}

	private static func getIOName(_ entry: io_registry_entry_t) -> String? {
		let pointer = UnsafeMutablePointer<io_name_t>.allocate(capacity: 1)
		defer { pointer.deallocate() }
		
		let result = IORegistryEntryGetName(entry, pointer)
		if result != kIOReturnSuccess {
			print("Error IORegistryEntryGetName(): " + (String(cString: mach_error_string(result), encoding: String.Encoding.ascii) ?? "unknown error"))
			return nil
		}
		
		return String(cString: UnsafeRawPointer(pointer).assumingMemoryBound(to: CChar.self))
	}

	private static func getCPUCores() -> (Int32?, Int32?, [CoreInfo])? {
		var iterator: io_iterator_t = io_iterator_t()
		let result = IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("AppleARMPE"), &iterator)
		if result != kIOReturnSuccess {
			print("Error find AppleARMPE: " + (String(cString: mach_error_string(result), encoding: String.Encoding.ascii) ?? "unknown error"))
			return nil
		}
		
		var service: io_registry_entry_t = 1
		var list: [CoreInfo] = []
		var pCores: Int32? = nil
		var eCores: Int32? = nil
		
		while service != 0 {
			service = IOIteratorNext(iterator)
			
			var entry: io_iterator_t = io_iterator_t()

			if IORegistryEntryGetChildIterator(service, kIOServicePlane, &entry) != kIOReturnSuccess {
				continue
			}
	
			var child: io_registry_entry_t = 1

			while child != 0 {
				child = IOIteratorNext(entry)
				guard child != 0 else {
					continue
				}
				
				guard let name = getIOName(child), let props = getIOProperties(child) else { continue }

				if name.matches("^cpu\\d") {
					var type = CoreInfo.CoreType.unknown
					
					if let rawType = props.object(forKey: "cluster-type") as? Data,
					   let typ = String(data: rawType, encoding: .utf8)?.trimmed {
						switch typ {
						case "E":
							type = .efficiency
						case "P":
							type = .performance
						default:
							type = .unknown
						}
					}
					
					let rawCPUId = props.object(forKey: "cpu-id") as? Data
					let id = rawCPUId?.withUnsafeBytes { pointer in
						return pointer.load(as: Int32.self)
					}
					
					list.append(CoreInfo(id: id ?? -1, type: type))
				} else if name.trimmed == "cpus" {
					eCores = (props.object(forKey: "e-core-count") as? Data)?.withUnsafeBytes { pointer in
						return pointer.load(as: Int32.self)
					}
					pCores = (props.object(forKey: "p-core-count") as? Data)?.withUnsafeBytes { pointer in
						return pointer.load(as: Int32.self)
					}
				}
				
				IOObjectRelease(child)
			}
			IOObjectRelease(entry)
			IOObjectRelease(service)
		}
		IOObjectRelease(iterator)
		
		return (eCores, pCores, list)
	}

	private static func convertCFDataToArr(_ data: CFData, _ isM4: Bool = false) -> [Int32] {
		let length = CFDataGetLength(data)
		var bytes = [UInt8](repeating: 0, count: length)
		CFDataGetBytes(data, CFRange(location: 0, length: length), &bytes)
		
		var multiplier: UInt32 = 1000 * 1000
		if isM4 {
			multiplier = 1000
		}
		
		var arr: [Int32] = []
		var chunks = stride(from: 0, to: bytes.count, by: 8).map { Array(bytes[$0..<min($0 + 8, bytes.count)])}
		for chunk in chunks {
			let v = UInt32(chunk[0]) | UInt32(chunk[1]) << 8 | UInt32(chunk[2]) << 16 | UInt32(chunk[3]) << 24
			arr.append(Int32(v / multiplier))
		}
		bytes.removeAll()
		chunks.removeAll()
		
		return arr
	}

	private static func getFrequencies(cpuName: String) -> ([Int32], [Int32])? {
		var iterator = io_iterator_t()

		guard IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("AppleARMIODevice"), &iterator) == kIOReturnSuccess else {
			return nil
		}

		let isM4: Bool = cpuName.lowercased().contains("m4")
		var eFreq: [Int32] = []
		var pFreq: [Int32] = []
		
		while case let child = IOIteratorNext(iterator), child != 0 {
			defer { IOObjectRelease(child) }
			guard let name = getIOName(child), name == "pmgr", let props = getIOProperties(child) else { continue }
			
			if let data = props.value(forKey: "voltage-states1-sram") {
				eFreq = convertCFDataToArr(data as! CFData, isM4)
			}
			if let data = props.value(forKey: "voltage-states5-sram") {
				pFreq = convertCFDataToArr(data as! CFData, isM4)
			}
		}
		
		return (eFreq, pFreq)
	}

	private static func getCPUInfo() -> CpuInformations? {
		let cpu = CpuInformations()
		var name: String = sysctlByName("machdep.cpu.brand_string")

		if name != "" {
			name = name.replacingOccurrences(of: "(TM)", with: "")
			name = name.replacingOccurrences(of: "(R)", with: "")
			name = name.replacingOccurrences(of: "CPU", with: "")
			name = name.replacingOccurrences(of: "@", with: "")
			
			cpu.name = name.condenseWhitespace()
		}
		
		var size = UInt32(MemoryLayout<host_basic_info_data_t>.size / MemoryLayout<integer_t>.size)
		let hostInfo = host_basic_info_t.allocate(capacity: 1)
		defer {
			hostInfo.deallocate()
		}
		
		let result = hostInfo.withMemoryRebound(to: integer_t.self, capacity: Int(size)) {
			host_info(mach_host_self(), HOST_BASIC_INFO, $0, &size)
		}
		
		if result != KERN_SUCCESS {
			print("read cores number: \(String(cString: mach_error_string(result), encoding: String.Encoding.ascii) ?? "unknown error")")
			return nil
		}
		
		let data = hostInfo.move()
		cpu.physicalCores = Int8(data.physical_cpu)
		cpu.logicalCores = Int8(data.logical_cpu)
		
		if let cores = getCPUCores() {
			cpu.eCores = cores.0
			cpu.pCores = cores.1
			cpu.cores = cores.2
		}

		if let freq = getFrequencies(cpuName: cpu.name ?? "") {
			cpu.eCoreFrequencies = freq.0
			cpu.pCoreFrequencies = freq.1
		}
		
		return cpu
	}

	private func hostCPULoadInfo() -> host_cpu_load_info? {
		let count = MemoryLayout<host_cpu_load_info>.stride/MemoryLayout<integer_t>.stride
		var size = mach_msg_type_number_t(count)
		var cpuLoadInfo = host_cpu_load_info()
		
		let result: kern_return_t = withUnsafeMutablePointer(to: &cpuLoadInfo) {
			$0.withMemoryRebound(to: integer_t.self, capacity: count) {
				host_statistics(mach_host_self(), HOST_CPU_LOAD_INFO, $0, &size)
			}
		}

		if result != KERN_SUCCESS {
			return nil
		}
		
		return cpuLoadInfo
	}

	public init() {
		self.hasHyperthreadingCores = sysctlByName("hw.physicalcpu") != sysctlByName("hw.logicalcpu")

		[CTL_HW, HW_NCPU].withUnsafeBufferPointer { mib in
			var sizeOfNumCPUs: size_t = MemoryLayout<uint>.size
			let status = sysctl(processor_info_array_t(mutating: mib.baseAddress), 2, &numCPUs, &sizeOfNumCPUs, nil, 0)
			if status != 0 {
				self.numCPUs = 1
			}
		}

		self.cores = Self.getCPUInfo()?.cores
		
		self.refresh()
	}

	public func refresh() {
		let result: kern_return_t = host_processor_info(mach_host_self(), PROCESSOR_CPU_LOAD_INFO, &self.numCPUsU, &self.cpuInfo, &self.numCpuInfo)
		guard result == KERN_SUCCESS else {
			return
		}

		guard let cpuInfo = hostCPULoadInfo() else {
			return
		}

		self.CPUUsageLock.lock()
		self.usagePerCore = []

		for i in 0 ..< Int32(numCPUs) {
			var inUse: Int32
			var total: Int32
			let user: Int32 = self.cpuInfo[Int(CPU_STATE_MAX * i + CPU_STATE_USER)]
			let system: Int32 = self.cpuInfo[Int(CPU_STATE_MAX * i + CPU_STATE_SYSTEM)]
			let nice: Int32 = self.cpuInfo[Int(CPU_STATE_MAX * i + CPU_STATE_NICE)]
			let idle: Int32 = self.cpuInfo[Int(CPU_STATE_MAX * i + CPU_STATE_IDLE)]

			if let prevCpuInfo = self.prevCpuInfo {
				let prev_user: Int32 = prevCpuInfo[Int(CPU_STATE_MAX * i + CPU_STATE_USER)]
				let prev_system: Int32 =  prevCpuInfo[Int(CPU_STATE_MAX * i + CPU_STATE_SYSTEM)]
				let prev_nice: Int32 = prevCpuInfo[Int(CPU_STATE_MAX * i + CPU_STATE_NICE)]
				let prev_idle: Int32 = prevCpuInfo[Int(CPU_STATE_MAX * i + CPU_STATE_IDLE)]

				inUse = (user - prev_user) + (system - prev_system) + (nice - prev_nice)
				total = inUse + (idle - prev_idle)
			} else {
				inUse = user + system + idle
				total = inUse + idle
			}
			
			self.usagePerCore.append(.init(coreID: i, user: user, system: system, idle: idle, nice: nice, usage: Double(inUse) / Double(total)))
		}
		
		if self.hasHyperthreadingCores == false {
			self.cpuLoad.usagePerCore = self.usagePerCore
		} else {
			var i = 0
			var a = 0
			
			self.cpuLoad.usagePerCore = []

			while i < Int(self.usagePerCore.count/2) {
				a = i*2

				if self.usagePerCore.indices.contains(a) && self.usagePerCore.indices.contains(a + 1) {
					let first = self.usagePerCore[a]
					let second = self.usagePerCore[a + 1]

					self.cpuLoad.usagePerCore.append(.init(coreID: Int32(i), user: (first.user + second.user) / 2, system: (first.system + second.system) / 2, idle: (first.idle + second.idle) / 2, nice: (first.nice + second.nice) / 2, usage: (first.usage + second.usage) / 2))
				}

				i += 1
			}
		}
		
		if let prevCpuInfo = self.prevCpuInfo {
			let prevCpuInfoSize: size_t = MemoryLayout<integer_t>.stride * Int(self.numPrevCpuInfo)
			vm_deallocate(mach_task_self_, vm_address_t(bitPattern: prevCpuInfo), vm_size_t(prevCpuInfoSize))
		}
		
		self.prevCpuInfo = self.cpuInfo
		self.numPrevCpuInfo = self.numCpuInfo
		self.cpuInfo = nil
		self.numCpuInfo = 0

		let userDiff = Double(cpuInfo.cpu_ticks.0 - self.previousInfo.cpu_ticks.0)
		let sysDiff  = Double(cpuInfo.cpu_ticks.1 - self.previousInfo.cpu_ticks.1)
		let idleDiff = Double(cpuInfo.cpu_ticks.2 - self.previousInfo.cpu_ticks.2)
		let niceDiff = Double(cpuInfo.cpu_ticks.3 - self.previousInfo.cpu_ticks.3)
		let totalTicks = sysDiff + userDiff + niceDiff + idleDiff
		let system = sysDiff / totalTicks
		let user = userDiff / totalTicks
		let idle = idleDiff / totalTicks
		
		if !system.isNaN {
			self.cpuLoad.systemLoad  = system
		}

		if !user.isNaN {
			self.cpuLoad.userLoad = user
		}

		if !idle.isNaN {
			self.cpuLoad.idleLoad = idle
		}

		self.previousInfo = cpuInfo
		self.cpuLoad.totalUsage = self.cpuLoad.systemLoad + self.cpuLoad.userLoad
		
		if let cores = self.cores {
			let eCoresList: [CoreLoad] = cores.filter({ $0.type == .efficiency }).enumerated().compactMap { (i, c) -> CoreLoad? in
				if self.cpuLoad.usagePerCore.indices.contains(Int(c.id)) {
					return self.cpuLoad.usagePerCore[Int(c.id)]
				}
				return i < usagePerCore.count ? usagePerCore[i] : nil
			}

			let pCoresList: [CoreLoad] = cores.filter({ $0.type == .performance }).enumerated().compactMap { (i, c) -> CoreLoad? in
				if self.cpuLoad.usagePerCore.indices.contains(Int(c.id)) {
					return self.cpuLoad.usagePerCore[Int(c.id)]
				}

				return i < usagePerCore.count ? usagePerCore[i] : nil
			}
			
			self.cpuLoad.usageECores = eCoresList.reduce(0, { $0 + $1.usage }) / Double(eCoresList.count)
			self.cpuLoad.usagePCores = pCoresList.reduce(0, { $0 + $1.usage }) / Double(pCoresList.count)
		}

		self.CPUUsageLock.unlock()
	}
}

