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
extension Cakeagent_ExecuteRequest {
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

final class CakeAgentProvider: Sendable, Cakeagent_AgentAsyncProvider {
	let group: EventLoopGroup
	let logger: Logger = Logging.Logger(label: "com.aldunelabs.cakeagent")

	init(group: EventLoopGroup) {
		self.group = group
	}

	func info(request: Google_Protobuf_Empty, context: GRPCAsyncServerCallContext) async throws -> Cakeagent_InfoReply {
		let processInfo = ProcessInfo.processInfo
		var reply = Cakeagent_InfoReply()
		var memory = Cakeagent_InfoReply.MemoryInfo()
		var size: size_t = 0
		var memSize: UInt64 = 0
		var freeMemory: UInt64 = 0

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

	func execute(requestStream: Cakeagent_ExecuteRequestStream, responseStream: Cakeagent_ExecuteResponseStream, context: GRPCAsyncServerCallContext) async throws {
		let process = ExecuteHandleStream(on: self.group.next())
		try await process.stream2(requestStream: requestStream, responseStream: responseStream)
	}

	func mount(request: Cakeagent_MountRequest, context: GRPC.GRPCAsyncServerCallContext) async throws -> Cakeagent_MountReply {
		Cakeagent_MountReply.with { $0.error = "Not supported" }
	}

	func umount(request: Cakeagent_MountRequest, context: GRPC.GRPCAsyncServerCallContext) async throws -> Cakeagent_MountReply {
		Cakeagent_MountReply.with { $0.error = "Not supported" }
	}

}	
