// DO NOT EDIT.
// swift-format-ignore-file
// swiftlint:disable all
//
// Generated by the Swift generator plugin for the protocol buffer compiler.
// Source: agent.proto
//
// For information on using the generated types, please see the documentation:
//   https://github.com/apple/swift-protobuf/

import Foundation
import SwiftProtobuf

// If the compiler emits an error on this type, it is because this file
// was generated by a version of the `protoc` Swift plug-in that is
// incompatible with the version of SwiftProtobuf to which you are linking.
// Please ensure that you are building against the same version of the API
// that was used to generate this file.
fileprivate struct _GeneratedWithProtocGenSwiftVersion: SwiftProtobuf.ProtobufAPIVersionCheck {
  struct _2: SwiftProtobuf.ProtobufAPIVersion_2 {}
  typealias Version = _2
}

public enum Cakeagent_Format: SwiftProtobuf.Enum, Swift.CaseIterable {
  public typealias RawValue = Int
  case stdout // = 0
  case stderr // = 1
  case end // = 2
  case UNRECOGNIZED(Int)

  public init() {
    self = .stdout
  }

  public init?(rawValue: Int) {
    switch rawValue {
    case 0: self = .stdout
    case 1: self = .stderr
    case 2: self = .end
    default: self = .UNRECOGNIZED(rawValue)
    }
  }

  public var rawValue: Int {
    switch self {
    case .stdout: return 0
    case .stderr: return 1
    case .end: return 2
    case .UNRECOGNIZED(let i): return i
    }
  }

  // The compiler won't synthesize support with the UNRECOGNIZED case.
  public static let allCases: [Cakeagent_Format] = [
    .stdout,
    .stderr,
    .end,
  ]

}

public struct Cakeagent_InfoReply: Sendable {
  // SwiftProtobuf.Message conformance is added in an extension below. See the
  // `Message` and `Message+*Additions` files in the SwiftProtobuf library for
  // methods supported on all messages.

  public var version: String = String()

  public var uptime: Int64 = 0

  public var memory: Cakeagent_InfoReply.MemoryInfo {
    get {return _memory ?? Cakeagent_InfoReply.MemoryInfo()}
    set {_memory = newValue}
  }
  /// Returns true if `memory` has been explicitly set.
  public var hasMemory: Bool {return self._memory != nil}
  /// Clears the value of `memory`. Subsequent reads from it will return its default value.
  public mutating func clearMemory() {self._memory = nil}

  public var cpuCount: Int32 = 0

  public var ipaddresses: [String] = []

  public var osname: String = String()

  public var hostname: String = String()

  public var release: String = String()

  public var unknownFields = SwiftProtobuf.UnknownStorage()

  public struct MemoryInfo: Sendable {
    // SwiftProtobuf.Message conformance is added in an extension below. See the
    // `Message` and `Message+*Additions` files in the SwiftProtobuf library for
    // methods supported on all messages.

    public var total: UInt64 = 0

    public var free: UInt64 = 0

    public var used: UInt64 = 0

    public var unknownFields = SwiftProtobuf.UnknownStorage()

    public init() {}
  }

  public init() {}

  fileprivate var _memory: Cakeagent_InfoReply.MemoryInfo? = nil
}

public struct Cakeagent_ExecuteRequest: @unchecked Sendable {
  // SwiftProtobuf.Message conformance is added in an extension below. See the
  // `Message` and `Message+*Additions` files in the SwiftProtobuf library for
  // methods supported on all messages.

  public var input: Data {
    get {return _input ?? Data()}
    set {_input = newValue}
  }
  /// Returns true if `input` has been explicitly set.
  public var hasInput: Bool {return self._input != nil}
  /// Clears the value of `input`. Subsequent reads from it will return its default value.
  public mutating func clearInput() {self._input = nil}

  public var command: String = String()

  public var args: [String] = []

  public var unknownFields = SwiftProtobuf.UnknownStorage()

  public init() {}

  fileprivate var _input: Data? = nil
}

public struct Cakeagent_ExecuteReply: @unchecked Sendable {
  // SwiftProtobuf.Message conformance is added in an extension below. See the
  // `Message` and `Message+*Additions` files in the SwiftProtobuf library for
  // methods supported on all messages.

  public var input: Data {
    get {return _input ?? Data()}
    set {_input = newValue}
  }
  /// Returns true if `input` has been explicitly set.
  public var hasInput: Bool {return self._input != nil}
  /// Clears the value of `input`. Subsequent reads from it will return its default value.
  public mutating func clearInput() {self._input = nil}

  public var output: Data {
    get {return _output ?? Data()}
    set {_output = newValue}
  }
  /// Returns true if `output` has been explicitly set.
  public var hasOutput: Bool {return self._output != nil}
  /// Clears the value of `output`. Subsequent reads from it will return its default value.
  public mutating func clearOutput() {self._output = nil}

  public var error: Data {
    get {return _error ?? Data()}
    set {_error = newValue}
  }
  /// Returns true if `error` has been explicitly set.
  public var hasError: Bool {return self._error != nil}
  /// Clears the value of `error`. Subsequent reads from it will return its default value.
  public mutating func clearError() {self._error = nil}

  public var exitCode: Int32 = 0

  public var unknownFields = SwiftProtobuf.UnknownStorage()

  public init() {}

  fileprivate var _input: Data? = nil
  fileprivate var _output: Data? = nil
  fileprivate var _error: Data? = nil
}

public struct Cakeagent_ShellMessage: @unchecked Sendable {
  // SwiftProtobuf.Message conformance is added in an extension below. See the
  // `Message` and `Message+*Additions` files in the SwiftProtobuf library for
  // methods supported on all messages.

  public var datas: Data = Data()

  public var unknownFields = SwiftProtobuf.UnknownStorage()

  public init() {}
}

public struct Cakeagent_ShellResponse: @unchecked Sendable {
  // SwiftProtobuf.Message conformance is added in an extension below. See the
  // `Message` and `Message+*Additions` files in the SwiftProtobuf library for
  // methods supported on all messages.

  public var format: Cakeagent_Format = .stdout

  public var datas: Data {
    get {return _datas ?? Data()}
    set {_datas = newValue}
  }
  /// Returns true if `datas` has been explicitly set.
  public var hasDatas: Bool {return self._datas != nil}
  /// Clears the value of `datas`. Subsequent reads from it will return its default value.
  public mutating func clearDatas() {self._datas = nil}

  public var unknownFields = SwiftProtobuf.UnknownStorage()

  public init() {}

  fileprivate var _datas: Data? = nil
}

// MARK: - Code below here is support for the SwiftProtobuf runtime.

fileprivate let _protobuf_package = "cakeagent"

extension Cakeagent_Format: SwiftProtobuf._ProtoNameProviding {
  public static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    0: .same(proto: "stdout"),
    1: .same(proto: "stderr"),
    2: .same(proto: "end"),
  ]
}

extension Cakeagent_InfoReply: SwiftProtobuf.Message, SwiftProtobuf._MessageImplementationBase, SwiftProtobuf._ProtoNameProviding {
  public static let protoMessageName: String = _protobuf_package + ".InfoReply"
  public static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    1: .same(proto: "version"),
    2: .same(proto: "uptime"),
    3: .same(proto: "memory"),
    4: .same(proto: "cpuCount"),
    5: .same(proto: "ipaddresses"),
    6: .same(proto: "osname"),
    7: .same(proto: "hostname"),
    8: .same(proto: "release"),
  ]

  public mutating func decodeMessage<D: SwiftProtobuf.Decoder>(decoder: inout D) throws {
    while let fieldNumber = try decoder.nextFieldNumber() {
      // The use of inline closures is to circumvent an issue where the compiler
      // allocates stack space for every case branch when no optimizations are
      // enabled. https://github.com/apple/swift-protobuf/issues/1034
      switch fieldNumber {
      case 1: try { try decoder.decodeSingularStringField(value: &self.version) }()
      case 2: try { try decoder.decodeSingularInt64Field(value: &self.uptime) }()
      case 3: try { try decoder.decodeSingularMessageField(value: &self._memory) }()
      case 4: try { try decoder.decodeSingularInt32Field(value: &self.cpuCount) }()
      case 5: try { try decoder.decodeRepeatedStringField(value: &self.ipaddresses) }()
      case 6: try { try decoder.decodeSingularStringField(value: &self.osname) }()
      case 7: try { try decoder.decodeSingularStringField(value: &self.hostname) }()
      case 8: try { try decoder.decodeSingularStringField(value: &self.release) }()
      default: break
      }
    }
  }

  public func traverse<V: SwiftProtobuf.Visitor>(visitor: inout V) throws {
    // The use of inline closures is to circumvent an issue where the compiler
    // allocates stack space for every if/case branch local when no optimizations
    // are enabled. https://github.com/apple/swift-protobuf/issues/1034 and
    // https://github.com/apple/swift-protobuf/issues/1182
    if !self.version.isEmpty {
      try visitor.visitSingularStringField(value: self.version, fieldNumber: 1)
    }
    if self.uptime != 0 {
      try visitor.visitSingularInt64Field(value: self.uptime, fieldNumber: 2)
    }
    try { if let v = self._memory {
      try visitor.visitSingularMessageField(value: v, fieldNumber: 3)
    } }()
    if self.cpuCount != 0 {
      try visitor.visitSingularInt32Field(value: self.cpuCount, fieldNumber: 4)
    }
    if !self.ipaddresses.isEmpty {
      try visitor.visitRepeatedStringField(value: self.ipaddresses, fieldNumber: 5)
    }
    if !self.osname.isEmpty {
      try visitor.visitSingularStringField(value: self.osname, fieldNumber: 6)
    }
    if !self.hostname.isEmpty {
      try visitor.visitSingularStringField(value: self.hostname, fieldNumber: 7)
    }
    if !self.release.isEmpty {
      try visitor.visitSingularStringField(value: self.release, fieldNumber: 8)
    }
    try unknownFields.traverse(visitor: &visitor)
  }

  public static func ==(lhs: Cakeagent_InfoReply, rhs: Cakeagent_InfoReply) -> Bool {
    if lhs.version != rhs.version {return false}
    if lhs.uptime != rhs.uptime {return false}
    if lhs._memory != rhs._memory {return false}
    if lhs.cpuCount != rhs.cpuCount {return false}
    if lhs.ipaddresses != rhs.ipaddresses {return false}
    if lhs.osname != rhs.osname {return false}
    if lhs.hostname != rhs.hostname {return false}
    if lhs.release != rhs.release {return false}
    if lhs.unknownFields != rhs.unknownFields {return false}
    return true
  }
}

extension Cakeagent_InfoReply.MemoryInfo: SwiftProtobuf.Message, SwiftProtobuf._MessageImplementationBase, SwiftProtobuf._ProtoNameProviding {
  public static let protoMessageName: String = Cakeagent_InfoReply.protoMessageName + ".MemoryInfo"
  public static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    1: .same(proto: "total"),
    2: .same(proto: "free"),
    3: .same(proto: "used"),
  ]

  public mutating func decodeMessage<D: SwiftProtobuf.Decoder>(decoder: inout D) throws {
    while let fieldNumber = try decoder.nextFieldNumber() {
      // The use of inline closures is to circumvent an issue where the compiler
      // allocates stack space for every case branch when no optimizations are
      // enabled. https://github.com/apple/swift-protobuf/issues/1034
      switch fieldNumber {
      case 1: try { try decoder.decodeSingularUInt64Field(value: &self.total) }()
      case 2: try { try decoder.decodeSingularUInt64Field(value: &self.free) }()
      case 3: try { try decoder.decodeSingularUInt64Field(value: &self.used) }()
      default: break
      }
    }
  }

  public func traverse<V: SwiftProtobuf.Visitor>(visitor: inout V) throws {
    if self.total != 0 {
      try visitor.visitSingularUInt64Field(value: self.total, fieldNumber: 1)
    }
    if self.free != 0 {
      try visitor.visitSingularUInt64Field(value: self.free, fieldNumber: 2)
    }
    if self.used != 0 {
      try visitor.visitSingularUInt64Field(value: self.used, fieldNumber: 3)
    }
    try unknownFields.traverse(visitor: &visitor)
  }

  public static func ==(lhs: Cakeagent_InfoReply.MemoryInfo, rhs: Cakeagent_InfoReply.MemoryInfo) -> Bool {
    if lhs.total != rhs.total {return false}
    if lhs.free != rhs.free {return false}
    if lhs.used != rhs.used {return false}
    if lhs.unknownFields != rhs.unknownFields {return false}
    return true
  }
}

extension Cakeagent_ExecuteRequest: SwiftProtobuf.Message, SwiftProtobuf._MessageImplementationBase, SwiftProtobuf._ProtoNameProviding {
  public static let protoMessageName: String = _protobuf_package + ".ExecuteRequest"
  public static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    1: .same(proto: "input"),
    2: .same(proto: "command"),
    3: .same(proto: "args"),
  ]

  public mutating func decodeMessage<D: SwiftProtobuf.Decoder>(decoder: inout D) throws {
    while let fieldNumber = try decoder.nextFieldNumber() {
      // The use of inline closures is to circumvent an issue where the compiler
      // allocates stack space for every case branch when no optimizations are
      // enabled. https://github.com/apple/swift-protobuf/issues/1034
      switch fieldNumber {
      case 1: try { try decoder.decodeSingularBytesField(value: &self._input) }()
      case 2: try { try decoder.decodeSingularStringField(value: &self.command) }()
      case 3: try { try decoder.decodeRepeatedStringField(value: &self.args) }()
      default: break
      }
    }
  }

  public func traverse<V: SwiftProtobuf.Visitor>(visitor: inout V) throws {
    // The use of inline closures is to circumvent an issue where the compiler
    // allocates stack space for every if/case branch local when no optimizations
    // are enabled. https://github.com/apple/swift-protobuf/issues/1034 and
    // https://github.com/apple/swift-protobuf/issues/1182
    try { if let v = self._input {
      try visitor.visitSingularBytesField(value: v, fieldNumber: 1)
    } }()
    if !self.command.isEmpty {
      try visitor.visitSingularStringField(value: self.command, fieldNumber: 2)
    }
    if !self.args.isEmpty {
      try visitor.visitRepeatedStringField(value: self.args, fieldNumber: 3)
    }
    try unknownFields.traverse(visitor: &visitor)
  }

  public static func ==(lhs: Cakeagent_ExecuteRequest, rhs: Cakeagent_ExecuteRequest) -> Bool {
    if lhs._input != rhs._input {return false}
    if lhs.command != rhs.command {return false}
    if lhs.args != rhs.args {return false}
    if lhs.unknownFields != rhs.unknownFields {return false}
    return true
  }
}

extension Cakeagent_ExecuteReply: SwiftProtobuf.Message, SwiftProtobuf._MessageImplementationBase, SwiftProtobuf._ProtoNameProviding {
  public static let protoMessageName: String = _protobuf_package + ".ExecuteReply"
  public static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    1: .same(proto: "input"),
    2: .same(proto: "output"),
    3: .same(proto: "error"),
    4: .same(proto: "exitCode"),
  ]

  public mutating func decodeMessage<D: SwiftProtobuf.Decoder>(decoder: inout D) throws {
    while let fieldNumber = try decoder.nextFieldNumber() {
      // The use of inline closures is to circumvent an issue where the compiler
      // allocates stack space for every case branch when no optimizations are
      // enabled. https://github.com/apple/swift-protobuf/issues/1034
      switch fieldNumber {
      case 1: try { try decoder.decodeSingularBytesField(value: &self._input) }()
      case 2: try { try decoder.decodeSingularBytesField(value: &self._output) }()
      case 3: try { try decoder.decodeSingularBytesField(value: &self._error) }()
      case 4: try { try decoder.decodeSingularInt32Field(value: &self.exitCode) }()
      default: break
      }
    }
  }

  public func traverse<V: SwiftProtobuf.Visitor>(visitor: inout V) throws {
    // The use of inline closures is to circumvent an issue where the compiler
    // allocates stack space for every if/case branch local when no optimizations
    // are enabled. https://github.com/apple/swift-protobuf/issues/1034 and
    // https://github.com/apple/swift-protobuf/issues/1182
    try { if let v = self._input {
      try visitor.visitSingularBytesField(value: v, fieldNumber: 1)
    } }()
    try { if let v = self._output {
      try visitor.visitSingularBytesField(value: v, fieldNumber: 2)
    } }()
    try { if let v = self._error {
      try visitor.visitSingularBytesField(value: v, fieldNumber: 3)
    } }()
    if self.exitCode != 0 {
      try visitor.visitSingularInt32Field(value: self.exitCode, fieldNumber: 4)
    }
    try unknownFields.traverse(visitor: &visitor)
  }

  public static func ==(lhs: Cakeagent_ExecuteReply, rhs: Cakeagent_ExecuteReply) -> Bool {
    if lhs._input != rhs._input {return false}
    if lhs._output != rhs._output {return false}
    if lhs._error != rhs._error {return false}
    if lhs.exitCode != rhs.exitCode {return false}
    if lhs.unknownFields != rhs.unknownFields {return false}
    return true
  }
}

extension Cakeagent_ShellMessage: SwiftProtobuf.Message, SwiftProtobuf._MessageImplementationBase, SwiftProtobuf._ProtoNameProviding {
  public static let protoMessageName: String = _protobuf_package + ".ShellMessage"
  public static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    1: .same(proto: "datas"),
  ]

  public mutating func decodeMessage<D: SwiftProtobuf.Decoder>(decoder: inout D) throws {
    while let fieldNumber = try decoder.nextFieldNumber() {
      // The use of inline closures is to circumvent an issue where the compiler
      // allocates stack space for every case branch when no optimizations are
      // enabled. https://github.com/apple/swift-protobuf/issues/1034
      switch fieldNumber {
      case 1: try { try decoder.decodeSingularBytesField(value: &self.datas) }()
      default: break
      }
    }
  }

  public func traverse<V: SwiftProtobuf.Visitor>(visitor: inout V) throws {
    if !self.datas.isEmpty {
      try visitor.visitSingularBytesField(value: self.datas, fieldNumber: 1)
    }
    try unknownFields.traverse(visitor: &visitor)
  }

  public static func ==(lhs: Cakeagent_ShellMessage, rhs: Cakeagent_ShellMessage) -> Bool {
    if lhs.datas != rhs.datas {return false}
    if lhs.unknownFields != rhs.unknownFields {return false}
    return true
  }
}

extension Cakeagent_ShellResponse: SwiftProtobuf.Message, SwiftProtobuf._MessageImplementationBase, SwiftProtobuf._ProtoNameProviding {
  public static let protoMessageName: String = _protobuf_package + ".ShellResponse"
  public static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    1: .same(proto: "format"),
    2: .same(proto: "datas"),
  ]

  public mutating func decodeMessage<D: SwiftProtobuf.Decoder>(decoder: inout D) throws {
    while let fieldNumber = try decoder.nextFieldNumber() {
      // The use of inline closures is to circumvent an issue where the compiler
      // allocates stack space for every case branch when no optimizations are
      // enabled. https://github.com/apple/swift-protobuf/issues/1034
      switch fieldNumber {
      case 1: try { try decoder.decodeSingularEnumField(value: &self.format) }()
      case 2: try { try decoder.decodeSingularBytesField(value: &self._datas) }()
      default: break
      }
    }
  }

  public func traverse<V: SwiftProtobuf.Visitor>(visitor: inout V) throws {
    // The use of inline closures is to circumvent an issue where the compiler
    // allocates stack space for every if/case branch local when no optimizations
    // are enabled. https://github.com/apple/swift-protobuf/issues/1034 and
    // https://github.com/apple/swift-protobuf/issues/1182
    if self.format != .stdout {
      try visitor.visitSingularEnumField(value: self.format, fieldNumber: 1)
    }
    try { if let v = self._datas {
      try visitor.visitSingularBytesField(value: v, fieldNumber: 2)
    } }()
    try unknownFields.traverse(visitor: &visitor)
  }

  public static func ==(lhs: Cakeagent_ShellResponse, rhs: Cakeagent_ShellResponse) -> Bool {
    if lhs.format != rhs.format {return false}
    if lhs._datas != rhs._datas {return false}
    if lhs.unknownFields != rhs.unknownFields {return false}
    return true
  }
}
