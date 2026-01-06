import TextTable
import Foundation
import ArgumentParser

extension Data {
	func toString() -> String {
		return String(decoding: self, as: UTF8.self)
	}
}

public enum Format: String, ExpressibleByArgument, CaseIterable, Sendable, Codable, EnumerableFlag {
	case text, json

	public private(set) static var allValueStrings: [String] = Format.allCases.map { "\($0)" }

	public func renderSingle<T>(style: TextTableStyle.Type = Style.grid, uppercased: Bool = true, _ data: T) -> String where T: Encodable {
		switch self {
		case .text:
			return renderList(style: style, uppercased: uppercased, [data])
		case .json:
			let encoder = JSONEncoder()
			encoder.outputFormatting = [.prettyPrinted, .withoutEscapingSlashes]
			return try! encoder.encode(data).toString()
		}
	}

	public func renderList<T>(style: TextTableStyle.Type = Style.grid, uppercased: Bool = true, _ data: [T]) -> String where T: Encodable {
		switch self {
		case .text:
			if data.count == 0 {
				return ""
			}
			let table = TextTable<T> { (item: T) in
				return Mirror(reflecting: item).children.enumerated()
					.map { (_, element) in
						let label = element.label ?? "<unknown>"
						return Column(title: uppercased ? label.uppercased() : label, value: element.value)
					}
			}

			return table.string(for: data, style: style)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
		case .json:
			let encoder = JSONEncoder()
			encoder.outputFormatting = [.prettyPrinted, .withoutEscapingSlashes]
			return try! encoder.encode(data).toString()
		}
	}

	public func render(_ data: InfoReply) -> String {
		struct IP: Codable{
			let ip: [String]
		}

		switch self {
			case .json:
				return self.renderSingle(data)
			case .text:
				var text = self.renderSingle(ShortInfoReply(from: data))

				if let memory = data.memory {
					text += "\n\nMemory usage:\n" + self.renderSingle(memory)
				}

				if let cpuInfo = data.cpuInfo {
					text += "\n\nCPU information:\n" + self.renderSingle(ShortCpuInformations(from: cpuInfo))

					if cpuInfo.cores.isEmpty == false {
						text += "\n" + self.renderList(cpuInfo.cores)
					}
				}

				if data.ipaddresses.isEmpty == false {
					text += "\n\nIP addresses:\n" + self.renderSingle(IP(ip: data.ipaddresses))
				}

				if let mounts: [String] = data.mounts {
					text += "\n\nMounts:\n" + self.renderList(mounts)
				}

				if data.diskInfos.isEmpty == false {
					text += "\n\nDisk infos:\n" + self.renderList(data.diskInfos)
				}

				if let attachedNetworks = data.attachedNetworks, attachedNetworks.isEmpty == false {
					text += "\n\nAttached networks:\n" + self.renderList(attachedNetworks)
				}

				if let tunnelInfos = data.tunnelInfos, tunnelInfos.isEmpty == false {
					text += "\n\nTunnels:\n" + self.renderList(tunnelInfos)
				}

				if let socketInfos = data.socketInfos, socketInfos.isEmpty == false {
					text += "\nSockets:\n" + self.renderList(socketInfos)
				}

				return text
		}
	}
}