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
		
		var networkInfos: [String:CakeAgent.InfoReply.NetworkInfo] = [:]
		var ifaddr: UnsafeMutablePointer<ifaddrs>? = nil
		
		if getifaddrs(&ifaddr) == 0, let firstAddr = ifaddr {
			var ptr = firstAddr
			
			while ptr.pointee.ifa_next != nil {
				let interface = ptr.pointee
				let addrFamily = interface.ifa_addr.pointee.sa_family
				let name = String(validatingUTF8: interface.ifa_name)!
				var networkInfo: CakeAgent.InfoReply.NetworkInfo
				
				if let infos = networkInfos[name] {
					networkInfo = infos
				} else {
					networkInfo = CakeAgent.InfoReply.NetworkInfo.with {
						$0.interface = name
					}
				}

				if addrFamily == UInt8(AF_LINK) && name != "lo0" {
					if let sdlPtr = UnsafePointer<sockaddr_dl>(OpaquePointer(interface.ifa_addr)) {
						let sdl = sdlPtr.pointee
						let nameLen = Int(sdl.sdl_nlen)
						let addrLen = Int(sdl.sdl_alen)
						
						let macAddress: String = withUnsafeBytes(of: sdl) { rawBuffer in
							// sdl_data starts after the fixed header fields; compute its base offset
							let sdlDataOffset = MemoryLayout.offset(of: \sockaddr_dl.sdl_data) ?? 0
							let macStart = sdlDataOffset + nameLen
							guard macStart + addrLen <= rawBuffer.count else { return "" }
							let macBytes = rawBuffer[macStart..<(macStart + addrLen)]
							return macBytes.map { String(format: "%02x", $0) }.joined(separator: ":")
						}

						networkInfo.macAddress = macAddress

						if let dataPtr = interface.ifa_data {
							let ifData = dataPtr.assumingMemoryBound(to: if_data.self).pointee
							networkInfo.bytesReceived = Int64(ifData.ifi_ibytes)
							networkInfo.bytesSent = Int64(ifData.ifi_obytes)
							networkInfo.packetsReceived = Int64(ifData.ifi_ipackets)
							networkInfo.packetsSent = Int64(ifData.ifi_opackets)
						}

						networkInfos[name] = networkInfo
					}
				}


				if (addrFamily == UInt8(AF_INET) || addrFamily == UInt8(AF_INET6)) && name != "lo0" {
					var hostname = [CChar](repeating: 0, count: Int(NI_MAXHOST))
		
					getnameinfo(interface.ifa_addr,
								socklen_t(interface.ifa_addr.pointee.sa_len),
								&hostname, socklen_t(hostname.count),
								nil, socklen_t(0), NI_NUMERICHOST)
					
					// Build numeric address string
					let baseAddress = String(cString: hostname)

					// Compute CIDR prefix length from netmask
					var prefixLength: Int = -1
					if let netmaskPtr = interface.ifa_netmask {
						let family = interface.ifa_addr.pointee.sa_family
						if family == UInt8(AF_INET) {
							// IPv4 netmask
							let nm = UnsafeRawPointer(netmaskPtr).assumingMemoryBound(to: sockaddr_in.self).pointee
							let mask = nm.sin_addr.s_addr
							// Count bits set to 1 in network byte order
							var m = UInt32(bigEndian: mask)
							var count = 0
							while m != 0 {
								count += Int(m & 1)
								m >>= 1
							}
							prefixLength = count
						} else if family == UInt8(AF_INET6) {
							// IPv6 netmask
							let nm6 = UnsafeRawPointer(netmaskPtr).assumingMemoryBound(to: sockaddr_in6.self).pointee
							let addr = nm6.sin6_addr
							// Count bits in 16 bytes
							withUnsafeBytes(of: addr) { bytes in
								var cnt = 0
								for b in bytes {
									var v = b
									while v != 0 {
										cnt += Int(v & 1)
										v >>= 1
									}
								}
								prefixLength = cnt
							}
						}
					}

					// Append CIDR if computed; otherwise keep base address
					let address: String
					if prefixLength >= 0 {
						address = "\(baseAddress)/\(prefixLength)"
					} else {
						address = baseAddress
					}
					
					if address.contains("%") == false {
						networkInfo.addresses.append(address)

						networkInfos[name] = networkInfo
					}
				}
				
				ptr = interface.ifa_next
			}
			
			freeifaddrs(ifaddr)
		}
		
		reply.networkInfos = networkInfos.values.sorted { $0.interface < $1.interface }
		
		// Collecter les informations CPU
		(reply.cpuCount, reply.cpu) = cpuWatcher.collect()
		
		return reply
	}
}
