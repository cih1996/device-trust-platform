import CryptoKit
import XCTest
@testable import DeviceTrust

final class DeviceTrustTests: XCTestCase {
    func testHexRoundTrip() {
        let data = Data([0, 1, 0xab, 0xff])
        XCTAssertEqual(Hex.encode(data), "0001abff")
        XCTAssertEqual(Hex.decode("0001abff"), data)
        XCTAssertNil(Hex.decode("abc"))
    }

    func testFrameLayout() throws {
        let server = P256.KeyAgreement.PrivateKey()
        let key = SigningKey(kind: .software, blob: P256.Signing.PrivateKey().rawRepresentation)
        let frame = try Frame.protect(kind: 2, challengeId: Data(repeating: 7, count: 24), key: key, nonce: Data(repeating: 1, count: 32),
                                      server: server.publicKey.derRepresentation, payload: Data("{}".utf8))
        XCTAssertEqual(Array(frame.prefix(5)), Array("DTP4".utf8) + [2])
        let length = frame[138..<142].reduce(0) { $0 << 8 | Int($1) }
        XCTAssertEqual(length, 2 + 16)
        let end = 142 + length
        let sigLength = Int(frame[end]) << 8 | Int(frame[end + 1])
        XCTAssertEqual(frame.count, end + 2 + sigLength)
        let signature = try P256.Signing.ECDSASignature(derRepresentation: frame[(end + 2)...])
        let verifier = try P256.Signing.PublicKey(derRepresentation: try key.spki)
        XCTAssertTrue(verifier.isValidSignature(signature, for: frame[..<end]))
    }
}
