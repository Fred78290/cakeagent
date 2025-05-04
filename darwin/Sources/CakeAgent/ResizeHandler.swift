import Foundation

enum ResizeDiskError: Error {
	case diskNotFound(_ message: String)
	case diskAlreadyResized
	case unableToRepairDisk(_ message: String)
	case noPartitionFound
	case tooManyPartitionFound
	case noDiskFound(_ message: String)
	case noVolumes(_ message: String)
	case resizeContainerFailed(_ message: String)
	case unexpectedError(_ message: String)
}

extension Data {
	var string: String {
		return String(data: self, encoding: .utf8) ?? ""
	}
}

struct ResizeHandler {
	static func decodeFromString<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
		let decoder = PropertyListDecoder()

		return try decoder.decode(T.self, from: data)
	}

	struct CandidatePartition {
		let DiskName:       String
		let DiskSizeUnused: Int64
		let PartitionName:  String
	}

	struct PhysicalStore: Codable {
		let APFSPhysicalStores: [String]

		enum CodingKeys: String, CodingKey {
			case APFSPhysicalStores = "APFSPhysicalStores"
		}

		init(from decoder: Decoder) throws {
			let container = try decoder.container(keyedBy: CodingKeys.self)

			self.APFSPhysicalStores = try container.decode([String].self, forKey: .APFSPhysicalStores)
		}

		init() throws {
			let result = try ResizeHandler.executeCommand(input: nil, "diskutil info -plist /") // This command will return a plist with APFSPhysicalStores

			if result.exitCode != 0 {
				throw ResizeDiskError.noDiskFound(result.stderr.string)
			}

			self = try ResizeHandler.decodeFromString(Self.self, from: result.stdout)

			if self.APFSPhysicalStores.isEmpty {
				throw ResizeDiskError.noVolumes("APFSPhysicalStores not found")
			}
		}
	}

	struct ListVolumes: Codable {
		let PhysicalDisks: [PhysicalDisk]

		enum CodingKeys: String, CodingKey {
			case PhysicalDisks = "AllDisksAndPartitions"
		}

		init(from decoder: Decoder) throws {
			let container = try decoder.container(keyedBy: CodingKeys.self)

			self.PhysicalDisks = try container.decode([PhysicalDisk].self, forKey: .PhysicalDisks)
		}

		init() throws {
			let result = try ResizeHandler.executeCommand(input: nil, "diskutil list -plist physical") // This command will return a plist with all disks and partitions

			if result.exitCode != 0 {
				throw ResizeDiskError.noVolumes(result.stderr.string)
			}

			self = try ResizeHandler.decodeFromString(Self.self, from: result.stdout)

			if self.PhysicalDisks.isEmpty {
				throw ResizeDiskError.noVolumes("PhysicalDisks not found")
			}
		}
	}

	struct PhysicalDisk: Codable {
		let DeviceIdentifier: String
		let Size:             Int64
		let Partitions:       [DiskPartition]

		enum PartitionsKeys: String, CodingKey {
			case DeviceIdentifier = "DeviceIdentifier"
			case Size             = "Size"
			case Partitions       = "Partitions"
		}

		init(from decoder: Decoder) throws {
			let container = try decoder.container(keyedBy: PartitionsKeys.self)

			self.DeviceIdentifier = try container.decode(String.self, forKey: .DeviceIdentifier)
			self.Size             = try container.decode(Int64.self, forKey: .Size)
			self.Partitions       = try container.decode([DiskPartition].self, forKey: .Partitions)
		}
	}

	struct DiskPartition: Codable {
		let DeviceIdentifier: String
		let Size:             Int64
		let Content:          String

		enum DiskPartitionKeys: String, CodingKey {
			case DeviceIdentifier = "DeviceIdentifier"
			case Size             = "Size"
			case Content          = "Content"
		}

		init(from decoder: Decoder) throws {
			let container = try decoder.container(keyedBy: DiskPartitionKeys.self)

			DeviceIdentifier = try container.decode(String.self, forKey: .DeviceIdentifier)
			Size             = try container.decode(Int64.self, forKey: .Size)
			Content          = try container.decode(String.self, forKey: .Content)
		}
	}

	static func executeCommand(input: String?, _ command: String) throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
		let task = Process()
		var stdout = Data()
		var stderr = Data()

		if #available(OSX 10.13, *) {
			task.executableURL = URL(fileURLWithPath: "/bin/bash")
		} else {
			task.launchPath = "/bin/bash"
		}

		task.arguments = ["-c", command]

		let stdoutPipe = Pipe()
		let stderrPipe = Pipe()

		task.standardOutput = stdoutPipe
		task.standardError = stderrPipe

		stdoutPipe.fileHandleForReading.readabilityHandler = { fileHandle in
			let data = fileHandle.availableData
			if data.count > 0 {
				stdout.append(data)
			}
		}

		stderrPipe.fileHandleForReading.readabilityHandler = { fileHandle in
			let data = fileHandle.availableData
			if data.count > 0 {
				stderr.append(data)
			}
		}

		if let input = input {
			let inputPipe = Pipe()

			inputPipe.fileHandleForWriting.writeabilityHandler = { handler in
				handler.write(input.data(using: .utf8)!)
			}

			task.standardInput = inputPipe
		}

		if #available(OSX 10.13, *) {
			try task.run()
		} else {
			task.launch()
		}

		task.waitUntilExit()

		return (stdout, stderr, task.terminationStatus)
	}

	static func resizeDisk() throws {
		let physicalStore = try PhysicalStore()
		let listVolumes = try ListVolumes()
		let candidatePartitions: [CandidatePartition] = listVolumes.PhysicalDisks.reduce(into: [CandidatePartition]()) { partialResult, physicalDisk in
			let unusedSize = physicalDisk.Size - (physicalDisk.Partitions.map { $0.Size }.reduce(0, +))

			partialResult.append(contentsOf: physicalDisk.Partitions.compactMap { partition in
				if partition.Content == "Apple_APFS" && physicalStore.APFSPhysicalStores.contains(partition.DeviceIdentifier) {
					return CandidatePartition(DiskName: physicalDisk.DeviceIdentifier, DiskSizeUnused: unusedSize, PartitionName: partition.DeviceIdentifier)
				} else {
					return nil
				}
			})
		}

		guard candidatePartitions.isEmpty == false else {
			throw ResizeDiskError.noPartitionFound
		}

		guard candidatePartitions.count == 1 else {
			throw ResizeDiskError.tooManyPartitionFound
		}

		let partition = candidatePartitions[0]

		guard partition.DiskSizeUnused > 4096*10 else {
			throw ResizeDiskError.diskAlreadyResized
		}

		var out = try ResizeHandler.executeCommand(input: "yes", "diskutil repairDisk \(partition.DiskName)")

		guard out.exitCode == 0 else {
			throw ResizeDiskError.unableToRepairDisk(out.stderr.string)
		}

		out = try ResizeHandler.executeCommand(input: "yes", "diskutil apfs resizeContainer \(partition.PartitionName) 0")

		guard out.exitCode == 0 else {
			throw ResizeDiskError.unableToRepairDisk(out.stderr.string)
		}
	}
}
