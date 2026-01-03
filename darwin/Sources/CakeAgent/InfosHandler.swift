import Foundation
import CakeAgentLib

struct InfosHandler {
	static func infos(cpuWatcher: CPUWatcher) -> CakeAgent.InfoReply {
		let processInfo = ProcessInfo.processInfo
		var reply = CakeAgent.InfoReply()
		let keys: [URLResourceKey]
		if #available(macOS 13.3, *) {
			keys = [.volumeNameKey, .volumeIsBrowsableKey, .volumeIsAutomountedKey, .volumeIsRemovableKey, .volumeIsInternalKey, .volumeLocalizedNameKey, .volumeIsRootFileSystemKey, .volumeTotalCapacityKey, .volumeAvailableCapacityKey, .volumeMountFromLocationKey, .volumeTypeNameKey]
		} else {
			keys = [.volumeNameKey, .volumeIsBrowsableKey, .volumeIsAutomountedKey, .volumeIsRemovableKey, .volumeIsInternalKey, .volumeLocalizedNameKey, .volumeIsRootFileSystemKey, .volumeTotalCapacityKey, .volumeAvailableCapacityKey]
		}
		let volumes = FileManager.default.mountedVolumeURLs(includingResourceValuesForKeys: keys, options: []) ?? []
		
		reply.agentVersion = CI.version
		reply.diskInfos = volumes.compactMap { volume in
			if let resourceValues = try? volume.resourceValues(forKeys: Set(keys)),
			   let volumeIsBrowsable = resourceValues.volumeIsBrowsable,
			   let volumeIsRootFileSystem = resourceValues.volumeIsRootFileSystem,
			   let volumeName = resourceValues.volumeName {
				
				if volumeIsBrowsable {
					return CakeAgent.InfoReply.DiskInfo.with {
						if volumeIsRootFileSystem {
							$0.mount = "/"
						} else {
							$0.mount = "/Volumes/\(volumeName)"
						}
						
						if #available(macOS 13.3, *) {
							$0.fsType = resourceValues.volumeTypeName ?? ""
							if let volumeMountFromLocation = resourceValues.volumeMountFromLocation {
								$0.device = "/dev/\(volumeMountFromLocation)"
							} else {
								$0.device = ""
							}
						}
						
						$0.size = UInt64(resourceValues.volumeTotalCapacity ?? 0)
						$0.free = UInt64(resourceValues.volumeAvailableCapacity ?? 0)
						$0.used = $0.size - $0.free
					}
				}
			}
			
			return nil
		}

		reply.memory = .with {
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
		
		reply.osname = "darwin"
		reply.release = "\(processInfo.operatingSystemVersion.majorVersion).\(processInfo.operatingSystemVersion.minorVersion).\(processInfo.operatingSystemVersion.patchVersion)"
		reply.version = processInfo.operatingSystemVersionString
		reply.hostname = processInfo.hostName
		
		var ipAddresses: [String] = []
		var ifaddr: UnsafeMutablePointer<ifaddrs>? = nil
		
		if getifaddrs(&ifaddr) == 0, let firstAddr = ifaddr {
			var ptr = firstAddr
			var hostname = [CChar](repeating: 0, count: Int(NI_MAXHOST))
			
			while ptr.pointee.ifa_next != nil {
				let interface = ptr.pointee
				let addrFamily = interface.ifa_addr.pointee.sa_family
				let name = String(validatingUTF8: interface.ifa_name)!
				
				if (addrFamily == UInt8(AF_INET) || addrFamily == UInt8(AF_INET6)) && name != "lo0" {
					getnameinfo(interface.ifa_addr,
								socklen_t(interface.ifa_addr.pointee.sa_len),
								&hostname, socklen_t(hostname.count),
								nil, socklen_t(0), NI_NUMERICHOST)
					
					let address = String(cString: hostname)
					
					if address.contains("%") == false {
						ipAddresses.append(address)
					}
				}
				
				ptr = interface.ifa_next
			}
			
			freeifaddrs(ifaddr)
		}
		
		reply.ipaddresses = ipAddresses.sorted()
		
		// Collecter les informations CPU
		(reply.cpuCount, reply.cpu) = cpuWatcher.collect()
		
		return reply
	}
}