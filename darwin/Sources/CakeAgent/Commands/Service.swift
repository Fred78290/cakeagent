import ArgumentParser
import Foundation

enum InstallError: Error {
	case failedToWriteLaunchAgentPlist(_ message: String)
	case failedToWriteServiceUnit(_ message: String)
	case caNotFound(_ message: String)
	case tlsKeyNotFound(_ message: String)
	case tlsCertNotFound(_ message: String)
	case tlsRequired
}

extension InstallError: CustomStringConvertible {
	var description: String {
		switch self {
		case .failedToWriteLaunchAgentPlist(let message):
			return "Failed to write launch agent plist: \(message)"
		case .failedToWriteServiceUnit(let message):
			return "Failed to write service unit: \(message)"
		case .caNotFound(let message):
			return "CA not found: \(message)"
		case .tlsKeyNotFound(let message):
			return "TLS key not found: \(message)"
		case .tlsCertNotFound(let message):
			return "TLS cert not found: \(message)"
		case .tlsRequired:
			return "CA, TLS key and TLS cert are required for secure connection"
		}
	}
}

struct Service: ParsableCommand {
	static let configuration = CommandConfiguration(abstract: "cake agent",
	                                                subcommands: [Install.self])

	struct LaunchAgent: Codable {
		let label: String
		let programArguments: [String]
		let keepAlive: [String:Bool]
		let runAtLoad: Bool
		let abandonProcessGroup: Bool
		let softResourceLimits: [String:Int]
		let environmentVariables: [String:String]
		let standardErrorPath: String
		let standardOutPath: String
		let processType: String

		enum CodingKeys: String, CodingKey {
			case label = "Label"
			case programArguments = "ProgramArguments"
			case keepAlive = "KeepAlive"
			case runAtLoad = "RunAtLoad"
			case abandonProcessGroup = "AbandonProcessGroup"
			case softResourceLimits = "SoftResourceLimits"
			case environmentVariables = "EnvironmentVariables"
			case standardErrorPath = "StandardErrorPath"
			case standardOutPath = "StandardOutPath"
			case processType = "ProcessType"
		}

		func write(to: URL) throws {
			let encoder = PropertyListEncoder()
			encoder.outputFormat = .xml

			let data = try encoder.encode(self)
			try data.write(to: to)
		}
	}

	struct Install : ParsableCommand { 
		static let configuration = CommandConfiguration(abstract: "Install cake agent as service")

		@Option(name: [.customLong("listen"), .customShort("l")], help: "Listen on address")
		var address: String?

		@Flag(name: [.customLong("insecure"), .customShort("i")], help: "don't use TLS")
		var insecure: Bool = false

		@Option(name: [.customLong("ca-cert"), .customShort("c")], help: "CA TLS certificate")
		var caCert: String?

		@Option(name: [.customLong("tls-cert"), .customShort("t")], help: "Client TLS certificate")
		var tlsCert: String?

		@Option(name: [.customLong("tls-key"), .customShort("k")], help: "Client private key")
		var tlsKey: String?

		#if os(Linux)
			// Linux-specific installation logic
			func install(arguments: [String]) throws {
				let service = """
				[Unit]
				Description=cake agent
				After=network.target

				[Service]
				Type=simple
				ExecStart=\(arguments.joined(separator: " "))
				Restart=on-failure
				Environment=PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin/:/sbin

				[Install]
				WantedBy=multi-user.target
				"""

				let serviceURL = URL(fileURLWithPath: "/etc/systemd/system/\(cakerSignature).service")

				if FileManager.default.fileExists(atPath: serviceURL.path) {
					throw InstallError.failedToWriteLaunchAgentPlist("File already exists")
				}

				do {
					try service.write(to: serviceURL, atomically: true, encoding: .ascii)

					try Process.run("systemctl", ["daemon-reload"]).waitUntilExit()
					try Process.run("systemctl", ["enable", "\(cakedSignature).service"]).waitUntilExit()
					try Process.run("systemctl", ["start", "\(cakedSignature).service"]).waitUntilExit()
				} catch {
					throw InstallError.failedToWriteServiceUnit(error.localizedDescription)
				}
			}
		#else
			func install(arguments: [String]) throws {
				let agentURL = URL(fileURLWithPath: "/Library/LaunchDaemons/\(cakerSignature).plist")

				if FileManager.default.fileExists(atPath: agentURL.path) {
					throw InstallError.failedToWriteLaunchAgentPlist("File already exists")
				}

				let agent = LaunchAgent(label: cakerSignature,
				                        programArguments: arguments,
				                        keepAlive: [
				                        	"SuccessfulExit" : false
				                        ],
				                        runAtLoad: true,
				                        abandonProcessGroup: true,
				                        softResourceLimits: [
				                        	"NumberOfFiles" : 4096
				                        ],
				                        environmentVariables: [
				                        	"PATH": "/usr/local/bin:/usr/bin:/bin:/usr/sbin/:/sbin",
				                        ],
				                        standardErrorPath: "/Library/Logs/cakeagent.log",
				                        standardOutPath: "/Library/Logs/cakeagent.log",
				                        processType: "Background")

				do {
					try agent.write(to: agentURL)
				} catch {
					throw InstallError.failedToWriteLaunchAgentPlist(error.localizedDescription)
				}
			}
		#endif

		func getListenAddress() throws -> String {
			if let address = self.address {
				return address
			}
			
			return "vsock://any:5000"
		}

		mutating func run() throws {
			let listenAddress: String = try getListenAddress()

			var arguments: [String] = [
				"cakeagent",
				"run",
				"--listen=\(listenAddress)"
			]

			if self.insecure == false {
				if let ca = caCert?.expandingTildeInPath, let key = tlsKey?.expandingTildeInPath, let cert = tlsCert?.expandingTildeInPath {
					if FileManager.default.fileExists(atPath: ca) == false {
						throw InstallError.caNotFound(ca)
					}

					if FileManager.default.fileExists(atPath: key) == false {
						throw InstallError.tlsKeyNotFound(key)
					}

					if FileManager.default.fileExists(atPath: cert) == false {
						throw InstallError.tlsCertNotFound(cert)
					}

					arguments.append("--tls-key=\(cert)")
					arguments.append("--tls-cert=\(key)")
					arguments.append("--ca-cert=\(ca)")
				} else {
					throw InstallError.tlsRequired
				}

			}

			try install(arguments: arguments)
		}
	}
}
