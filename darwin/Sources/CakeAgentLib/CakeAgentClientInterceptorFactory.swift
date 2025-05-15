import Foundation
import GRPC
import NIO
import SwiftProtobuf

public final class CakeAgentClientInterceptorFactory: CakeAgentClientInterceptorState, CakeAgentServiceClientInterceptorFactoryProtocol {
	public typealias CakeAgentClientInterceptorFactoryCallback = @Sendable (_ method: GRPCMethodDescriptor) -> Bool

	internal let inputHandle: FileHandle
	internal let state: termios
	internal let callback: CakeAgentClientInterceptorFactoryCallback?

	public class CakeAgentClientInterceptor<Request, Response>: ClientInterceptor<Request, Response>, @unchecked Sendable {
		let state: termios
		let inputHandle: FileHandle

		public init(inputHandle: FileHandle, state: termios) {
			self.state = state
			self.inputHandle = FileHandle.standardInput
			super.init()
		}

		public func restoreState() {
			var state = self.state

			try? inputHandle.restoreState(&state)
		}

		func printError(_ error: Error) {
			var error = error
			let description: String

			if let status: any GRPCStatusTransformable = error as? GRPCStatusTransformable {
				error = status.makeGRPCStatus()
			}

			if let err: GRPCStatus = error as? GRPCStatus {
				if err.code == .unavailable || err.code == .cancelled{
					description = "Connection refused"
				} else {
					description = err.description
				}
			} else {
				description = error.localizedDescription
			}

			FileHandle.standardError.write(Data("\(description)\n".utf8))
		}

		public override func errorCaught(_ error: Error, context: ClientInterceptorContext<Request, Response>) {
			self.restoreState()

			printError((error))
			super.errorCaught(error, context: context)
			Foundation.exit(1)
		}

		public override func cancel(promise: EventLoopPromise<Void>?, context: ClientInterceptorContext<Request, Response>) {
			self.restoreState()

			FileHandle.standardError.write(Data("canceled\n".utf8))
			super.cancel(promise: promise, context: context)
			Foundation.exit(1)
		}
	}

	internal init(inputHandle: FileHandle, state: termios, callback: CakeAgentClientInterceptorFactoryCallback? = nil) {
		self.inputHandle = inputHandle
		self.state = state
		self.callback = callback
	}

	public init?(inputHandle: FileHandle, callback: CakeAgentClientInterceptorFactoryCallback? = nil) throws {
		guard inputHandle.isTTY() else {
			return nil
		}

		self.inputHandle = inputHandle
		self.state = try inputHandle.getState()
		self.callback = callback
	}

	public func restoreState() {
		var state = self.state

		try? inputHandle.restoreState(&state)
	}

	public func makeResizeDiskInterceptors() -> [ClientInterceptor<CakeAgent.Empty, CakeAgent.ResizeReply>] {
		if callback?(CakeAgentServiceClientMetadata.Methods.resizeDisk) == false {
			return []
		}

		return [CakeAgentClientInterceptor<CakeAgent.Empty, CakeAgent.ResizeReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeInfoInterceptors() -> [ClientInterceptor<CakeAgent.Empty, CakeAgent.InfoReply>] {
		if callback?(CakeAgentServiceClientMetadata.Methods.info) == false {
			return []
		}

		return [CakeAgentClientInterceptor<CakeAgent.Empty, CakeAgent.InfoReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeShutdownInterceptors() -> [ClientInterceptor<CakeAgent.Empty, CakeAgent.RunReply>] {
		if callback?(CakeAgentServiceClientMetadata.Methods.info) == false {
			return []
		}

		return [CakeAgentClientInterceptor<CakeAgent.Empty, CakeAgent.RunReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeRunInterceptors() -> [ClientInterceptor<CakeAgent.RunCommand, CakeAgent.RunReply>] {
		if callback?(CakeAgentServiceClientMetadata.Methods.run) == false {
			return []
		}

		return [CakeAgentClientInterceptor<CakeAgent.RunCommand, CakeAgent.RunReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeExecuteInterceptors() -> [ClientInterceptor<CakeAgent.ExecuteRequest, CakeAgent.ExecuteResponse>] {
		if callback?(CakeAgentServiceClientMetadata.Methods.execute) == false {
			return []
		}

		return [CakeAgentClientInterceptor<CakeAgent.ExecuteRequest, CakeAgent.ExecuteResponse>(inputHandle: inputHandle, state: state)]
	}

	public func makeMountInterceptors() -> [ClientInterceptor<CakeAgent.MountRequest, CakeAgent.MountReply>] {
		if callback?(CakeAgentServiceClientMetadata.Methods.mount) == false {
			return []
		}

		return [CakeAgentClientInterceptor<CakeAgent.MountRequest, CakeAgent.MountReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeUmountInterceptors() -> [ClientInterceptor<CakeAgent.MountRequest, CakeAgent.MountReply>] {
		if callback?(CakeAgentServiceClientMetadata.Methods.umount) == false {
			return []
		}

		return [CakeAgentClientInterceptor<CakeAgent.MountRequest, CakeAgent.MountReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeTunnelInterceptors() -> [ClientInterceptor<CakeAgent.TunnelMessage, CakeAgent.TunnelMessage>] {
		if callback?(CakeAgentServiceClientMetadata.Methods.tunnel) == false {
			return []
		}

		return [CakeAgentClientInterceptor<CakeAgent.TunnelMessage, CakeAgent.TunnelMessage>(inputHandle: inputHandle, state: state)]
	}
	

}
