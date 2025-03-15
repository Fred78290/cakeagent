import Foundation
import GRPC
import NIO
import SwiftProtobuf

public final class CakeAgentClientInterceptorFactory: CakeAgentClientInterceptorState, Cakeagent_AgentClientInterceptorFactoryProtocol {
	internal let inputHandle: FileHandle
	internal let state: termios

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

			inputHandle.restoreState(&state)
		}

		func printError(_ error: Error) {
			let description: String

			if let err: GRPCStatus = error as? GRPCStatus {
				if err.code == .unavailable || err.code == .cancelled{
					description = "closed connection"
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

	internal init(inputHandle: FileHandle, state: termios) {
		self.inputHandle = inputHandle
		self.state = state
	}

	public init?(inputHandle: FileHandle) {
		guard inputHandle.isTTY() else {
			return nil
		}

		self.inputHandle = inputHandle
		self.state = inputHandle.getState()
	}

	public func restoreState() {
		var state = self.state

		inputHandle.restoreState(&state)
	}

	public func makeInfoInterceptors() -> [ClientInterceptor<Google_Protobuf_Empty, Cakeagent_InfoReply>] {
		[CakeAgentClientInterceptor<Google_Protobuf_Empty, Cakeagent_InfoReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeRunInterceptors() -> [ClientInterceptor<Cakeagent_RunCommand, Cakeagent_RunReply>] {
		[CakeAgentClientInterceptor<Cakeagent_RunCommand, Cakeagent_RunReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeExecuteInterceptors() -> [ClientInterceptor<Cakeagent_ExecuteRequest, Cakeagent_ExecuteResponse>] {
		[CakeAgentClientInterceptor<Cakeagent_ExecuteRequest, Cakeagent_ExecuteResponse>(inputHandle: inputHandle, state: state)]
	}

	public func makeMountInterceptors() -> [ClientInterceptor<Cakeagent_MountRequest, Cakeagent_MountReply>] {
		[CakeAgentClientInterceptor<Cakeagent_MountRequest, Cakeagent_MountReply>(inputHandle: inputHandle, state: state)]
	}

	public func makeUmountInterceptors() -> [ClientInterceptor<Cakeagent_MountRequest, Cakeagent_MountReply>] {
		[CakeAgentClientInterceptor<Cakeagent_MountRequest, Cakeagent_MountReply>(inputHandle: inputHandle, state: state)]
	}

}
