@preconcurrency import CakeAgentLib
import GRPC
import NIO
import NIOCore
import SwiftProtobuf
import Foundation
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
	let cpuWatcher: CPUWatcher = .init()

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
		return InfosHandler.infos(cpuWatcher: self.cpuWatcher)
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
	
	func tunnel(requestStream: CakeAgentTunnelRequestStream, responseStream: CakeAgentTunnelResponseStream, context: GRPCAsyncServerCallContext) async throws {
		try await TunnelHandlerStream(on: self.group.next(), requestStream: requestStream, responseStream: responseStream).stream()
	}
	
	func mount(request: CakeAgent.MountRequest, context: GRPCAsyncServerCallContext) async throws -> CakeAgent.MountReply {
		CakeAgent.MountReply.with { $0.error = "Not supported" }
	}
	
	func umount(request: CakeAgent.MountRequest, context: GRPCAsyncServerCallContext) async throws -> CakeAgent.MountReply {
		CakeAgent.MountReply.with { $0.error = "Not supported" }
	}
	
	func events(request: CakeAgent.Empty, responseStream: GRPCAsyncResponseStreamWriter<CakeAgent.TunnelPortForwardEvent>, context: GRPCAsyncServerCallContext) async throws {
		try await responseStream.send(CakeAgent.TunnelPortForwardEvent.with { $0.error = "Not supported" })
	}
	
	func ping(request: CakeAgent.PingRequest, context: GRPCAsyncServerCallContext) async throws -> CakeAgent.PingReply {
		.with {
			$0.message = request.message
			$0.requestTimestamp = request.timestamp
			$0.responseTimestamp = Int64(Date().timeIntervalSince1970 * 1_000_000_000)
		}
	}
	
	func currentUsage(request: CakeAgent.CurrentUsageRequest, responseStream: GRPCAsyncResponseStreamWriter<CakeAgent.CurrentUsageReply>, context: GRPCAsyncServerCallContext) async throws {
		var frequency = UInt64(request.frequency)

		if frequency == 0 {
			frequency = 1
		}

		frequency = 1_000_000_000 / frequency

		// Stream des informations d'usage CPU en temps réel
		while true {
			do {
				// Collecter les informations CPU
				// Envoyer les données CPU via le stream
				try await responseStream.send(self.cpuWatcher.currentUsage())

				// Attendre 1 seconde avant la prochaine collecte
				try await Task.sleep(nanoseconds: frequency)
				
			} catch {
				logger.error("Erreur lors de la collecte des informations CPU : \(error)")
				break
			}
		}
	}
	
}
