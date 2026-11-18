import Foundation

/// Device registration and signed risk reports for the DeviceTrust platform.
///
/// The SDK only collects and signs; every decision is made on the server.
/// Call the methods from a task, never block the main thread on them.
public enum DeviceTrust {
    public static let version = "0.1.0"

    public struct Config: Sendable {
        /// Public app ID from the console.
        public let appId: String
        /// Your business server; it implements POST /device/prepare.
        public let businessURL: URL
        /// Base64 SHA-256 of a SubjectPublicKeyInfo in the server's TLS chain.
        public let pins: [String]

        public init(appId: String, businessURL: URL, pins: [String]) {
            precondition(!appId.isEmpty, "public app ID required")
            precondition(!pins.isEmpty, "SPKI certificate pins required")
            self.appId = appId
            self.businessURL = businessURL
            self.pins = pins
        }
    }

    public struct Result: Sendable {
        public let reportId: String
        public let deviceId: String
        public let keyId: String
        /// app_attest when Apple vouched for the device and app, else signature_only.
        public let proofLevel: String
    }

    /// Registers the device (first launch) or refreshes its record. Call it on
    /// every cold start once the user accepted the privacy policy.
    public static func initialize(config: Config, consent: Bool) async throws -> Result {
        guard consent else { throw DeviceTrustError.consentRequired }
        return try await Serial.shared.run { try await register(config: config) }
    }

    private static func register(config: Config) async throws -> Result {
        let report = try await Collector.collect(appId: config.appId)
        let keyId = try SecureReporter.keyId(appId: config.appId)
        let input: [String: Any] = ["appId": config.appId, "installationId": report["installationId"] as Any, "keyId": keyId]
        let prepared = try await PinnedHTTP.post(config.businessURL.appendingPathComponent("device/prepare"), pins: config.pins,
                                                 contentType: "application/json", body: try JSONSerialization.data(withJSONObject: input))
        guard prepared["appId"] as? String == config.appId,
              let platform = (prepared["platformUrl"] as? String).flatMap(URL.init(string:)),
              let challenge = prepared["challenge"] as? [String: Any] else {
            throw DeviceTrustError.protocolError("invalid /device/prepare response")
        }
        let result = try await SecureReporter.upload(platformURL: platform, pins: config.pins, challenge: challenge, report: report)
        Store.setLastReport(appId: config.appId, Date())
        return result
    }

    /// Refreshes the record when the last report is older than `interval`;
    /// call it when the app returns to the foreground. Returns nil when skipped.
    @discardableResult
    public static func refreshIfNeeded(config: Config, consent: Bool, interval: TimeInterval = 30 * 60) async throws -> Result? {
        guard consent else { throw DeviceTrustError.consentRequired }
        return try await Serial.shared.run {
            if let last = Store.lastReport(appId: config.appId), Date().timeIntervalSince(last) < interval { return nil }
            return try await register(config: config)
        }
    }

    /// Signs a report for a business challenge. Your server obtained the
    /// challenge from POST /v1/challenges and passes it, with the platform
    /// URL, to the app; give the returned reportId back to your server.
    public static func report(challenge: [String: Any], platformURL: URL, pins: [String]) async throws -> Result {
        guard let appId = challenge["appId"] as? String else { throw DeviceTrustError.protocolError("challenge without appId") }
        return try await Serial.shared.run {
            let report = try await Collector.collect(appId: appId)
            let result = try await SecureReporter.upload(platformURL: platformURL, pins: pins, challenge: challenge, report: report)
            Store.setLastReport(appId: appId, Date())
            return result
        }
    }

    /// The enrolled device key ID, or "" before the first registration.
    public static func keyId(appId: String) throws -> String { try SecureReporter.keyId(appId: appId) }

    /// The device ID issued by the platform, or "" before the first report.
    public static func deviceId(appId: String) -> String { Store.deviceId(appId: appId) }

    /// Removes the device key; the next call enrolls a new one.
    public static func reset(appId: String) throws { try SecureReporter.reset(appId: appId) }
}

public enum DeviceTrustError: Error, CustomStringConvertible {
    case consentRequired
    case protocolError(String)
    case http(Int, String)
    case pinMismatch
    case keyMismatch
    case hardwareRequired(String)
    case keychain(OSStatus)

    public var description: String {
        switch self {
        case .consentRequired: return "consent required before device registration"
        case .protocolError(let m): return "protocol error: \(m)"
        case .http(let code, let body): return "HTTP \(code): \(body)"
        case .pinMismatch: return "TLS SPKI pin mismatch"
        case .keyMismatch: return "local device key differs from server challenge"
        case .hardwareRequired(let m): return "hardware attestation required: \(m)"
        case .keychain(let status): return "keychain error \(status)"
        }
    }
}
