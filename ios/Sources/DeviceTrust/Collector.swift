import CryptoKit
import DeviceTrustCore
import Foundation
import GameController
import UIKit

/// Collects the report signals. Nothing here prompts for a permission.
/// Signal names shared with Android keep the same meaning and shape.
enum Collector {
    static func collect(appId: String) async throws -> [String: Any] {
        var s = Signals()
        let installation = Hex.encode(Data(SHA256.hash(data: try Store.deviceSecret() + Data(appId.utf8))))
        s.available("platform", "ios")
        s.available("keychainId", installation)
        s.available("manufacturer", "Apple")
        s.available("model", machine())
        s.available("memory", ["totalBytes": ProcessInfo.processInfo.physicalMemory])
        s.available("locale", ["language": Locale.preferredLanguages.first ?? "", "region": Locale.current.regionCode ?? "",
                               "timeZone": TimeZone.current.identifier])
        s.merge(await MainActor.run { foreground() })
        application(&s)
        s.available("behavior", Behavior.snapshot())
        runtime(&s)
        network(&s)
        return ["appId": appId, "installationId": installation, "package": Bundle.main.bundleIdentifier ?? "", "signals": s.values]
    }

    /// Hardware model identifier such as iPhone17,1.
    static func machine() -> String {
        #if targetEnvironment(simulator)
        return ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"] ?? "simulator"
        #else
        var info = utsname()
        uname(&info)
        return withUnsafeBytes(of: &info.machine) { String(decoding: $0.prefix { $0 != 0 }, as: UTF8.self) }
        #endif
    }

    @MainActor
    private static func foreground() -> Signals {
        var s = Signals()
        let device = UIDevice.current
        s.available("osInfo", ["system": device.systemName, "release": device.systemVersion])
        if let vendor = device.identifierForVendor { s.available("vendorId", vendor.uuidString) } else { s.status("vendorId", "unavailable", "device locked") }
        let screen = UIScreen.main
        s.available("display", ["widthPixels": Int(screen.nativeBounds.width), "heightPixels": Int(screen.nativeBounds.height),
                                "density": screen.nativeScale, "brightness": Double(screen.brightness)])
        let monitoring = device.isBatteryMonitoringEnabled
        device.isBatteryMonitoringEnabled = true
        if device.batteryLevel >= 0 {
            let plugged = device.batteryState == .charging || device.batteryState == .full
            s.available("battery", ["level": Int(device.batteryLevel * 100), "scale": 100, "plugged": plugged ? 1 : 0])
        } else {
            s.status("battery", "unavailable", "battery state unknown")
        }
        device.isBatteryMonitoringEnabled = monitoring
        // Assistive features that can drive the screen, and attached input
        // devices: hardware auto-clickers and farm controllers present as HID.
        s.available("iosAccessibility", ["switchControl": UIAccessibility.isSwitchControlRunning,
                                         "assistiveTouch": UIAccessibility.isAssistiveTouchRunning,
                                         "voiceOver": UIAccessibility.isVoiceOverRunning,
                                         "guidedAccess": UIAccessibility.isGuidedAccessEnabled])
        s.available("peripherals", ["keyboard": GCKeyboard.coalesced != nil, "mice": GCMouse.mice().count,
                                    "controllers": GCController.controllers().count])
        return s
    }

    private static func application(_ s: inout Signals) {
        let info = Bundle.main.infoDictionary ?? [:]
        // Install source: an embedded provisioning profile means a development,
        // ad hoc, enterprise or re-signed build; App Store apps have none.
        let provisioned = Bundle.main.path(forResource: "embedded", ofType: "mobileprovision") != nil
        let installer = provisioned ? "provisioned" : Bundle.main.appStoreReceiptURL?.lastPathComponent == "sandboxReceipt" ? "testflight" : "appstore"
        s.available("application", ["package": Bundle.main.bundleIdentifier ?? "", "versionName": info["CFBundleShortVersionString"] as? String ?? "",
                                    "versionCode": info["CFBundleVersion"] as? String ?? "", "installer": installer,
                                    "cryptid": Int(dtc_cryptid()), "sdkVersion": DeviceTrust.version])
    }

    private static func runtime(_ s: inout Signals) {
        switch dtc_debugger_attached() {
        case 1: s.available("debugger", true)
        case 0: s.available("debugger", false)
        default: s.status("debugger", "error", "sysctl failed")
        }
        #if targetEnvironment(simulator)
        s.available("emulatorHint", true)
        #else
        s.available("emulatorHint", false)
        #endif
        // The same files checked through libc (C core) and Foundation: a hook
        // that hides them from one layer shows up as a disagreement.
        let native = lines { dtc_jailbreak_paths($0, $1) }
        let candidates = lines { dtc_jailbreak_candidates($0, $1) }
        let foundation = candidates.filter { FileManager.default.fileExists(atPath: $0) }
        s.available("jailbreak", ["nativePaths": native, "foundationPaths": foundation, "sandboxEscape": dtc_sandbox_escape() == 1])
        s.available("injection", ["hookImages": lines { dtc_hook_images($0, $1) }, "foreignImages": Array(lines { dtc_foreign_images($0, $1) }.prefix(20)),
                                  "dyldInsert": dtc_dyld_insert() == 1, "fridaPort": dtc_frida_port() == 1])
    }

    private static func network(_ s: inout Signals) {
        guard let settings = CFNetworkCopySystemProxySettings()?.takeRetainedValue() as? [String: Any] else {
            s.status("httpProxy", "error", "proxy settings unavailable")
            s.status("vpn", "error", "proxy settings unavailable")
            return
        }
        let proxy = (settings["HTTPEnable"] as? Int ?? 0) == 1 || (settings["HTTPSEnable"] as? Int ?? 0) == 1 || settings["ProxyAutoConfigURLString"] != nil
        s.available("httpProxy", proxy)
        let scoped = (settings["__SCOPED__"] as? [String: Any])?.keys.map { $0 } ?? []
        s.available("vpn", scoped.contains { $0.hasPrefix("utun") || $0.hasPrefix("ipsec") || $0.hasPrefix("ppp") || $0.hasPrefix("tap") || $0.hasPrefix("tun") })
    }

    private static func lines(_ read: (UnsafeMutablePointer<CChar>, Int) -> Int32) -> [String] {
        var buffer = [CChar](repeating: 0, count: 8192)
        _ = buffer.withUnsafeMutableBufferPointer { read($0.baseAddress!, $0.count) }
        let text = String(cString: buffer)
        return text.isEmpty ? [] : text.components(separatedBy: "\n")
    }
}

struct Signals: @unchecked Sendable {
    private(set) var values: [String: Any] = [:]
    mutating func merge(_ other: Signals) { values.merge(other.values) { _, new in new } }
    mutating func available(_ name: String, _ value: Any) { values[name] = ["status": "available", "value": value] }
    mutating func status(_ name: String, _ status: String, _ reason: String) { values[name] = ["status": status, "reason": reason] }
}
