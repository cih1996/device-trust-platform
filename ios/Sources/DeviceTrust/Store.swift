import Foundation
import Security

/// Keychain items survive app reinstall on the same device (ThisDeviceOnly:
/// never synced or restored to another device). Small app state lives in
/// UserDefaults.
enum Store {
    private static let service = "com.devicetrust.sdk"

    static func keychainData(_ account: String) throws -> Data? {
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service,
                                    kSecAttrAccount as String: account, kSecReturnData as String: true]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess else { throw DeviceTrustError.keychain(status) }
        return item as? Data
    }

    static func setKeychainData(_ account: String, _ data: Data) throws {
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service,
                                    kSecAttrAccount as String: account]
        SecItemDelete(query as CFDictionary)
        var add = query
        add[kSecValueData as String] = data
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let status = SecItemAdd(add as CFDictionary, nil)
        guard status == errSecSuccess else { throw DeviceTrustError.keychain(status) }
    }

    static func deleteKeychain(_ account: String) throws {
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service,
                                    kSecAttrAccount as String: account]
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else { throw DeviceTrustError.keychain(status) }
    }

    /// A random per-device secret kept in the keychain; it survives reinstall.
    static func deviceSecret() throws -> Data {
        if let data = try keychainData("device-secret") { return data }
        var bytes = [UInt8](repeating: 0, count: 32)
        guard SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes) == errSecSuccess else { throw DeviceTrustError.keychain(errSecAllocate) }
        let data = Data(bytes)
        try setKeychainData("device-secret", data)
        return data
    }

    static func deviceId(appId: String) -> String { UserDefaults.standard.string(forKey: "dtp.deviceId." + appId) ?? "" }
    static func setDeviceId(appId: String, _ id: String) { UserDefaults.standard.set(id, forKey: "dtp.deviceId." + appId) }
    static func lastReport(appId: String) -> Date? { UserDefaults.standard.object(forKey: "dtp.lastReport." + appId) as? Date }
    static func setLastReport(appId: String, _ date: Date) { UserDefaults.standard.set(date, forKey: "dtp.lastReport." + appId) }
}
