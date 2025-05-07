import CakeAgentLib
import GRPC
import NIO
import NIOCore
import SwiftProtobuf
import Foundation
import Logging
import Darwin
import Semaphore
import Synchronization

let cakerSignature = "com.aldunelabs.cakeagent"

public typealias CakeAgent = Cakeagent_CakeAgent
public typealias CakeAgentServiceAsyncProvider = Cakeagent_CakeAgentServiceAsyncProvider

class ServiceError : Error, CustomStringConvertible, @unchecked Sendable {
	let description: String
	let exitCode: Int32

	init(_ what: String, _ code: Int32 = -1) {
		self.description = what
		self.exitCode = code
	}
}

extension TaskGroup<Void>: @retroactive @unchecked Sendable {

}

extension String {
	var expandingTildeInPath: String {
		if self.hasPrefix("~") {
			return NSString(string: self).expandingTildeInPath
		}

		return self
	}

	func shell() -> String {
		guard let shell = ProcessInfo.processInfo.environment["SHELL"] else {
			return self
		}

		return shell
	}

	func lookupPath() -> String {
		let paths = ProcessInfo.processInfo.environment["PATH"]?.components(separatedBy: ":") ?? []
		for path in paths {
			let fullPath = (path as NSString).appendingPathComponent(self)
			if FileManager.default.isExecutableFile(atPath: fullPath) {
				return fullPath
			}
		}

		return self
	}
}
extension CakeAgent.ExecuteRequest {
	var isCommand: Bool {
		guard case .command = self.request else {
			return false
		}

		return true
	}

	var isInput: Bool {
		guard case .input = self.request else {
			return false
		}

		return true
	}
}

final class CakeAgentProvider: Sendable, CakeAgentServiceAsyncProvider {
	let group: EventLoopGroup
	let logger: Logger = Logger("CakeAgentProvider")

	init(group: EventLoopGroup) {
		self.group = group
	}

	func resizeDisk(request: CakeAgent.Empty, context: GRPCAsyncServerCallContext) async throws -> CakeAgent.ResizeReply {
		do {
			try ResizeHandler.resizeDisk()

			return CakeAgent.ResizeReply.with {
				$0.success = true
			}
		} catch {
			return CakeAgent.ResizeReply.with {
				$0.failure = error.localizedDescription
			}
		}
	}

	func info(request: CakeAgent.Empty, context: GRPCAsyncServerCallContext) async throws -> CakeAgent.InfoReply {
		let processInfo = ProcessInfo.processInfo
		var reply = CakeAgent.InfoReply()
		var memory = CakeAgent.InfoReply.MemoryInfo()
		var size: size_t = 0
		var memSize: UInt64 = 0
		var freeMemory: UInt64 = 0
		let keys: [URLResourceKey]
		if #available(macOS 13.3, *) {
			keys = [.volumeNameKey, .volumeIsBrowsableKey, .volumeIsAutomountedKey, .volumeIsRemovableKey, .volumeIsInternalKey, .volumeLocalizedNameKey, .volumeIsRootFileSystemKey, .volumeTotalCapacityKey, .volumeAvailableCapacityKey, .volumeMountFromLocationKey, .volumeTypeNameKey]
		} else {
			keys = [.volumeNameKey, .volumeIsBrowsableKey, .volumeIsAutomountedKey, .volumeIsRemovableKey, .volumeIsInternalKey, .volumeLocalizedNameKey, .volumeIsRootFileSystemKey, .volumeTotalCapacityKey, .volumeAvailableCapacityKey]
		}
		let volumes = FileManager.default.mountedVolumeURLs(includingResourceValuesForKeys: keys, options: []) ?? []
		
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

		size = MemoryLayout<UInt64>.size
		sysctlbyname("hw.memsize", &memSize, &size, nil, 0)

		size = MemoryLayout<UInt64>.size
		sysctlbyname("vm.page_free_count", &freeMemory, &size, nil, 0)

		memory.free = freeMemory * UInt64(vm_page_size)
		memory.total = processInfo.physicalMemory
		memory.used = memory.total - memory.free

		reply.cpuCount = Int32(processInfo.processorCount)
		reply.memory = memory

		reply.osname = processInfo.operatingSystemVersionString
		reply.release = "\(processInfo.operatingSystemVersion.majorVersion).\(processInfo.operatingSystemVersion.minorVersion).\(processInfo.operatingSystemVersion.patchVersion)"
		reply.hostname = processInfo.hostName

		var ipAddresses: [String] = []
		var ifaddr: UnsafeMutablePointer<ifaddrs>? = nil

		if getifaddrs(&ifaddr) == 0, let firstAddr = ifaddr {
			var ptr = firstAddr

			while ptr.pointee.ifa_next != nil {
				let interface = ptr.pointee
				let addrFamily = interface.ifa_addr.pointee.sa_family

				if addrFamily == UInt8(AF_INET) || addrFamily == UInt8(AF_INET6) {
					var hostname = [CChar](repeating: 0, count: Int(NI_MAXHOST))

					getnameinfo(interface.ifa_addr,
					            socklen_t(interface.ifa_addr.pointee.sa_len),
					            &hostname, socklen_t(hostname.count),
					            nil, socklen_t(0), NI_NUMERICHOST)

					let address = String(cString: hostname)

					ipAddresses.append(address)
				}

				ptr = interface.ifa_next
			}

			freeifaddrs(ifaddr)
		}

		reply.ipaddresses = ipAddresses

		return reply
	}

	func shutdown(request: CakeAgent.Empty, context: GRPCAsyncServerCallContext) async throws -> CakeAgent.RunReply {
		let process = Process()
		let arguments: [String] = ["shutdown", "-h", "+1s"]

		logger.info("execute shutdown")

		process.executableURL = URL(fileURLWithPath: "/bin/sh")
		process.arguments = ["-c", arguments.joined(separator: " ")]
		process.standardInput = FileHandle.nullDevice
		process.standardOutput = FileHandle.nullDevice
		process.standardError = FileHandle.nullDevice
		process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser

		do {
			try process.run()

			return .init()
		} catch {
			logger.error("Failed to run shutdown command: \(error)")

			return CakeAgent.RunReply.with { reply in
				reply.stderr = error.localizedDescription.data(using: .utf8) ?? Data()
				reply.stdout = Data()
				if process.isRunning == false {
					reply.exitCode = Int32(process.terminationStatus)
				}
			}
		}
	}


	func run(request: CakeAgent.RunCommand, context: GRPCAsyncServerCallContext) async throws -> CakeAgent.RunReply {
		let process = Process()
		let outputPipe = Pipe()
		let errorPipe = Pipe()
		var outputData = Data()
		var errorData = Data()
		var arguments: [String] = [request.command.command]

		arguments.append(contentsOf: request.command.args)

		let outputQueue = DispatchQueue(label: "bash-output-queue")

		logger.info("execute \(request.command)")

		process.executableURL = URL(fileURLWithPath: "/bin/sh")
		process.arguments = ["-c", arguments.joined(separator: " ")]
		process.standardOutput = outputPipe
		process.standardError = errorPipe
		process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser

		if request.hasInput {
			let inputPipe = Pipe()

			process.standardInput = inputPipe

			inputPipe.fileHandleForWriting.writeabilityHandler = { handler in
				handler.write(request.input)
			}	
		} else {
			process.standardInput = FileHandle.nullDevice
		}

		outputPipe.fileHandleForReading.readabilityHandler = { handler in
			let data = handler.availableData

			if data.isEmpty == false {
				self.logger.info("outputPipe data \(String(data: data, encoding: .utf8) ?? "")")
				outputQueue.async {
					outputData.append(data)
				}
			}
		}

		errorPipe.fileHandleForReading.readabilityHandler = { handler in
			let data = handler.availableData

			if data.isEmpty == false {
				self.logger.info("errorPipe data \(String(data: data, encoding: .utf8) ?? "")")
				outputQueue.async {
					errorData.append(data)
				}
			}
		}

		try process.run()

		process.waitUntilExit()

		return CakeAgent.RunReply.with { reply in
			if outputData.isEmpty == false {
				reply.stdout = outputData
			}

			if errorData.isEmpty == false {
				reply.stderr = errorData
			}

			reply.exitCode = Int32(process.terminationStatus)
		}
	}

	func execute(requestStream: CakeAgentExecuteRequestStream, responseStream: CakeAgentExecuteResponseStream, context: GRPCAsyncServerCallContext) async throws {
		try await ExecuteHandleStream(on: self.group.next(), requestStream: requestStream, responseStream: responseStream).stream()
	}

	func mount(request: CakeAgent.MountRequest, context: GRPC.GRPCAsyncServerCallContext) async throws -> CakeAgent.MountReply {
		CakeAgent.MountReply.with { $0.error = "Not supported" }
	}

	func umount(request: CakeAgent.MountRequest, context: GRPC.GRPCAsyncServerCallContext) async throws -> CakeAgent.MountReply {
		CakeAgent.MountReply.with { $0.error = "Not supported" }
	}

}	
