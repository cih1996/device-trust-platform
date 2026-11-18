import CryptoKit
import DeviceCheck
import Foundation

/// The device signing key: a Secure Enclave P-256 key on devices, a software
/// key where the Secure Enclave is unavailable (Simulator). The server learns
/// which one through App Attest, never from this flag.
struct SigningKey: Codable {
    enum Kind: String, Codable { case enclave, software }
    let kind: Kind
    let blob: Data            // Secure Enclave handle or software raw key
    var appAttestKeyId: String?

    static func generate() throws -> SigningKey {
        if SecureEnclave.isAvailable {
            return SigningKey(kind: .enclave, blob: try SecureEnclave.P256.Signing.PrivateKey().dataRepresentation)
        }
        return SigningKey(kind: .software, blob: P256.Signing.PrivateKey().rawRepresentation)
    }

    var spki: Data {
        get throws {
            switch kind {
            case .enclave: return try SecureEnclave.P256.Signing.PrivateKey(dataRepresentation: blob).publicKey.derRepresentation
            case .software: return try P256.Signing.PrivateKey(rawRepresentation: blob).publicKey.derRepresentation
            }
        }
    }

    var id: String { get throws { Hex.encode(Data(SHA256.hash(data: try spki))) } }

    func sign(_ data: Data) throws -> Data {
        switch kind {
        case .enclave: return try SecureEnclave.P256.Signing.PrivateKey(dataRepresentation: blob).signature(for: data).derRepresentation
        case .software: return try P256.Signing.PrivateKey(rawRepresentation: blob).signature(for: data).derRepresentation
        }
    }
}

enum SecureReporter {
    static let frameInfo = Data("device-trust/frame/v1".utf8)
    static let attestLabel = Data("device-trust/appattest/v1".utf8)
    static let assertionLabel = Data("device-trust/assertion/v1".utf8)

    private static func account(_ appId: String) -> String { "key." + Hex.encode(Data(SHA256.hash(data: Data(appId.utf8)))) }

    static func load(appId: String) throws -> SigningKey? {
        guard let data = try Store.keychainData(account(appId)) else { return nil }
        return try JSONDecoder().decode(SigningKey.self, from: data)
    }

    static func keyId(appId: String) throws -> String { try load(appId: appId)?.id ?? "" }

    static func reset(appId: String) throws { try Store.deleteKeychain(account(appId)) }

    static func upload(platformURL: URL, pins: [String], challenge: [String: Any], report: [String: Any]) async throws -> DeviceTrust.Result {
        guard let appId = challenge["appId"] as? String, appId == report["appId"] as? String else {
            throw DeviceTrustError.protocolError("report application mismatch")
        }
        guard let challengeHex = challenge["id"] as? String, let challengeId = Hex.decode(challengeHex), challengeId.count == 24,
              let nonce = (challenge["nonce"] as? String).flatMap({ Data(base64Encoded: $0) }),
              let server = (challenge["serverPublicKey"] as? String).flatMap({ Data(base64Encoded: $0) }),
              let expected = challenge["keyId"] as? String else {
            throw DeviceTrustError.protocolError("invalid challenge")
        }
        let hardwareRequired = challenge["hardwareRequired"] as? Bool ?? false
        let endpoint = platformURL.appendingPathComponent("v1/secure")
        var key: SigningKey
        if expected.isEmpty {
            key = try SigningKey.generate()
            let spki = try key.spki
            var body: [String: Any] = ["platform": "ios", "publicKey": spki.base64EncodedString()]
            do {
                let (attestKeyId, attestation) = try await appAttest(clientData: attestLabel + nonce + spki)
                key.appAttestKeyId = attestKeyId
                body["appAttest"] = ["keyId": attestKeyId, "attestation": attestation.base64EncodedString()]
            } catch {
                if hardwareRequired { throw DeviceTrustError.hardwareRequired(String(describing: error)) }
                body["attestationError"] = String(String(describing: error).prefix(500))
            }
            let enrollment = try JSONSerialization.data(withJSONObject: body)
            let frame = try Frame.protect(kind: 1, challengeId: challengeId, key: key, nonce: nonce, server: server, payload: enrollment)
            _ = try await PinnedHTTP.post(endpoint, pins: pins, contentType: "application/octet-stream", body: frame)
            try Store.setKeychainData(account(appId), try JSONEncoder().encode(key))
        } else {
            guard let stored = try load(appId: appId), try stored.id == expected else { throw DeviceTrustError.keyMismatch }
            key = stored
        }
        var payload = report
        if let attestKeyId = key.appAttestKeyId {
            let clientData = assertionLabel + nonce + (Hex.decode(try key.id) ?? Data())
            do {
                payload["appAttest"] = try await DCAppAttestService.shared.generateAssertion(attestKeyId, clientDataHash: Data(SHA256.hash(data: clientData))).base64EncodedString()
            } catch let error as DCError where error.code == .invalidKey {
                // The App Attest key is gone (e.g. restored to another device): enroll afresh next time.
                try reset(appId: appId)
                throw error
            }
        }
        let frame = try Frame.protect(kind: 2, challengeId: challengeId, key: key, nonce: nonce, server: server,
                                      payload: try JSONSerialization.data(withJSONObject: payload))
        let result = try await PinnedHTTP.post(endpoint, pins: pins, contentType: "application/octet-stream", body: frame)
        guard let reportId = result["reportId"] as? String, let deviceId = result["deviceId"] as? String,
              let keyId = result["keyId"] as? String, let proof = result["proofLevel"] as? String else {
            throw DeviceTrustError.protocolError("invalid report response")
        }
        Store.setDeviceId(appId: appId, deviceId)
        return DeviceTrust.Result(reportId: reportId, deviceId: deviceId, keyId: keyId, proofLevel: proof)
    }

    private static func appAttest(clientData: Data) async throws -> (String, Data) {
        let service = DCAppAttestService.shared
        guard service.isSupported else { throw DeviceTrustError.protocolError("App Attest unsupported") }
        let keyId = try await service.generateKey()
        let attestation = try await service.attestKey(keyId, clientDataHash: Data(SHA256.hash(data: clientData)))
        return (keyId, attestation)
    }
}

/// DTP4 frame, identical to the Android SDK: ECDH P-256 with the challenge's
/// server key, HKDF-SHA256 (salt = challenge nonce), AES-256-GCM with the
/// 142-byte header as AAD, then an ECDSA signature over header and ciphertext.
enum Frame {
    static func protect(kind: UInt8, challengeId: Data, key: SigningKey, nonce: Data, server: Data, payload: Data) throws -> Data {
        let serverKey = try P256.KeyAgreement.PublicKey(derRepresentation: server)
        let ephemeral = P256.KeyAgreement.PrivateKey()
        let secret = try ephemeral.sharedSecretFromKeyAgreement(with: serverKey)
        let aesKey = secret.hkdfDerivedSymmetricKey(using: SHA256.self, salt: nonce, sharedInfo: SecureReporter.frameInfo, outputByteCount: 32)
        let gcmNonce = AES.GCM.Nonce()
        guard let keyId = Hex.decode(try key.id) else { throw DeviceTrustError.protocolError("key ID") }
        var header = Data("DTP4".utf8)
        header.append(kind)
        header.append(challengeId)
        header.append(keyId)
        header.append(ephemeral.publicKey.x963Representation)
        header.append(contentsOf: gcmNonce)
        let length = UInt32(payload.count + 16)
        header.append(contentsOf: [UInt8(length >> 24), UInt8(length >> 16 & 0xff), UInt8(length >> 8 & 0xff), UInt8(length & 0xff)])
        precondition(header.count == 142)
        let sealed = try AES.GCM.seal(payload, using: aesKey, nonce: gcmNonce, authenticating: header)
        var signed = header
        signed.append(sealed.ciphertext)
        signed.append(sealed.tag)
        let signature = try key.sign(signed)
        signed.append(contentsOf: [UInt8(signature.count >> 8), UInt8(signature.count & 0xff)])
        signed.append(signature)
        return signed
    }
}

enum Hex {
    static func encode(_ data: Data) -> String { data.map { String(format: "%02x", $0) }.joined() }
    static func decode(_ text: String) -> Data? {
        guard text.count % 2 == 0 else { return nil }
        var out = Data(capacity: text.count / 2)
        var index = text.startIndex
        while index < text.endIndex {
            let next = text.index(index, offsetBy: 2)
            guard let byte = UInt8(text[index..<next], radix: 16) else { return nil }
            out.append(byte)
            index = next
        }
        return out
    }
}
