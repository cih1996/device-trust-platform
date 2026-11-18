import CryptoKit
import Foundation
import Security

/// HTTPS POST with the platform trust evaluation plus SHA-256 SPKI pinning:
/// the connection is accepted only when a certificate of the validated chain
/// has a pinned public key. Redirects are refused.
enum PinnedHTTP {
    static func post(_ url: URL, pins: [String], contentType: String, body: Data) async throws -> [String: Any] {
        guard url.scheme == "https" else { throw DeviceTrustError.protocolError("HTTPS required") }
        let expected = try pins.map { pin -> Data in
            guard let data = Data(base64Encoded: pin), data.count == 32 else { throw DeviceTrustError.protocolError("SHA-256 SPKI pin required") }
            return data
        }
        var request = URLRequest(url: url, timeoutInterval: 15)
        request.httpMethod = "POST"
        request.setValue(contentType, forHTTPHeaderField: "Content-Type")
        request.httpBody = body
        let delegate = PinDelegate(pins: expected)
        let session = URLSession(configuration: .ephemeral, delegate: delegate, delegateQueue: nil)
        defer { session.finishTasksAndInvalidate() }
        let data: Data, response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            if delegate.mismatch { throw DeviceTrustError.pinMismatch }
            throw error
        }
        guard let http = response as? HTTPURLResponse else { throw DeviceTrustError.protocolError("not an HTTP response") }
        guard data.count <= 512 * 1024 else { throw DeviceTrustError.protocolError("response exceeds 512 KiB") }
        let text = String(decoding: data, as: UTF8.self)
        guard (200..<300).contains(http.statusCode) else { throw DeviceTrustError.http(http.statusCode, text) }
        guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw DeviceTrustError.protocolError("response is not a JSON object")
        }
        return object
    }
}

private final class PinDelegate: NSObject, URLSessionTaskDelegate {
    let pins: [Data]
    private(set) var mismatch = false
    init(pins: [Data]) { self.pins = pins }

    func urlSession(_ session: URLSession, task: URLSessionTask, didReceive challenge: URLAuthenticationChallenge,
                    completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        guard challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
              let trust = challenge.protectionSpace.serverTrust else {
            completionHandler(.performDefaultHandling, nil)
            return
        }
        var error: CFError?
        guard SecTrustEvaluateWithError(trust, &error),
              let chain = SecTrustCopyCertificateChain(trust) as? [SecCertificate] else {
            completionHandler(.cancelAuthenticationChallenge, nil)
            return
        }
        for certificate in chain {
            if let spki = DER.subjectPublicKeyInfo(SecCertificateCopyData(certificate) as Data),
               pins.contains(Data(SHA256.hash(data: spki))) {
                completionHandler(.useCredential, URLCredential(trust: trust))
                return
            }
        }
        mismatch = true
        completionHandler(.cancelAuthenticationChallenge, nil)
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}

/// Just enough DER to find a certificate's SubjectPublicKeyInfo.
enum DER {
    /// Returns (tag, header length, content length) of the element at offset.
    static func element(_ b: [UInt8], _ at: Int) -> (tag: UInt8, header: Int, length: Int)? {
        guard at + 2 <= b.count else { return nil }
        let tag = b[at]
        var length = Int(b[at + 1])
        var header = 2
        if length & 0x80 != 0 {
            let count = length & 0x7f
            guard count >= 1, count <= 4, at + 2 + count <= b.count else { return nil }
            length = 0
            for i in 0..<count { length = length << 8 | Int(b[at + 2 + i]) }
            header += count
        }
        guard at + header + length <= b.count else { return nil }
        return (tag, header, length)
    }

    static func subjectPublicKeyInfo(_ certificate: Data) -> Data? {
        let b = [UInt8](certificate)
        guard let cert = element(b, 0), cert.tag == 0x30,
              let tbs = element(b, cert.header), tbs.tag == 0x30 else { return nil }
        var at = cert.header + tbs.header
        let end = at + tbs.length
        if let version = element(b, at), version.tag == 0xa0 { at += version.header + version.length }
        // serialNumber, signature, issuer, validity, subject, then subjectPublicKeyInfo.
        for _ in 0..<5 {
            guard let e = element(b, at) else { return nil }
            at += e.header + e.length
        }
        guard at < end, let spki = element(b, at), spki.tag == 0x30 else { return nil }
        return Data(b[at..<(at + spki.header + spki.length)])
    }
}
