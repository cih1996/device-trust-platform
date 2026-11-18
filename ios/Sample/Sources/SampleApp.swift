import DeviceTrust
import SwiftUI

/// The online experience app: registers the device, signs in to the demo
/// business and runs protected operations whose results show in the console.
enum Demo {
    static let appId = "d9235deaa49a812797ce7978780925889a232da958183d6a"
    static let business = URL(string: "https://devicetrust-sdk-api.lingzidev.com/demo")!
    static let pins = ["AU7LGDAFJUgs4HkcpRf1O7/6OU9avVMvle6wTsAK5m4="]
    static let password = Bundle.main.object(forInfoDictionaryKey: "DTPDemoPassword") as? String ?? ""
    static let console = "https://devicetrust-console.lingzidev.com/"
    static let consoleAccount = "demo-ios", consolePassword = "zhiheng2026"
    static var config: DeviceTrust.Config { DeviceTrust.Config(appId: appId, businessURL: business, pins: pins) }
}

@main
struct SampleApp: App {
    @Environment(\.scenePhase) private var phase
    @StateObject private var model = SampleModel()

    var body: some Scene {
        WindowGroup { ContentView().environmentObject(model) }
            .onChange(of: phase) { phase in
                if phase == .active { model.refresh() }
            }
    }
}

@MainActor
final class SampleModel: ObservableObject {
    @Published var consent = UserDefaults.standard.bool(forKey: "consent") {
        didSet {
            UserDefaults.standard.set(consent, forKey: "consent")
            if consent { register() }
        }
    }
    @Published var account = "体验用户"
    @Published var password = Demo.password
    @Published var status = "准备登记设备"
    @Published var busy = false
    private var token = "", platform: URL?, eventId = ""

    func register() { run { "设备已登记\n" + self.describe(try await DeviceTrust.initialize(config: Demo.config, consent: true)) } }

    func refresh() {
        guard consent else { return }
        run {
            guard let result = try await DeviceTrust.refreshIfNeeded(config: Demo.config, consent: true) else { return self.status }
            return "设备档案已更新\n" + self.describe(result)
        }
    }

    func login() {
        run {
            let result = try await self.post("login", ["accountId": self.account, "password": self.password])
            self.token = result["token"] as? String ?? ""
            self.platform = (result["platformUrl"] as? String).flatMap(URL.init(string:))
            return "已登录 · \(self.account)"
        }
    }

    func operate(_ kind: String) {
        guard !token.isEmpty, let platform else { status = "请先登录"; return }
        guard consent else { status = "请先同意采集"; return }
        run {
            let prepared = try await self.post("prepare", ["type": kind, "keyId": try DeviceTrust.keyId(appId: Demo.appId)])
            guard let eventId = prepared["eventId"] as? String, let challenge = prepared["challenge"] as? [String: Any] else {
                throw DeviceTrustError.protocolError("invalid prepare response")
            }
            self.eventId = eventId
            let report = try await DeviceTrust.report(challenge: challenge, platformURL: platform, pins: Demo.pins)
            return try await self.complete(reportId: report.reportId) + "\n" + self.describe(report)
        }
    }

    private func complete(reportId: String) async throws -> String {
        let result = try await post("complete", ["eventId": eventId, "reportId": reportId])
        switch result["state"] as? String {
        case "completed": return "操作已完成"
        case "verify": return "需要核验 · 请在工作台处理后继续"
        case "hold": return "等待审核 · 请在工作台处理后继续"
        default: throw DeviceTrustError.protocolError("未知业务状态")
        }
    }

    private func describe(_ r: DeviceTrust.Result) -> String {
        "设备 ID：\(r.deviceId)\n证明：\(r.proofLevel == "app_attest" ? "App Attest 硬件证明" : "仅签名")"
    }

    private func post(_ path: String, _ body: [String: Any]) async throws -> [String: Any] {
        var request = URLRequest(url: Demo.business.appendingPathComponent(path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if !token.isEmpty { request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization") }
        request.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (data, response) = try await URLSession.shared.data(for: request)
        let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] ?? [:]
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw DeviceTrustError.protocolError(object["error"] as? String ?? "HTTP 错误")
        }
        return object
    }

    private func run(_ work: @escaping () async throws -> String) {
        busy = true
        Task {
            do { status = try await work() } catch { status = "失败：\(error)" }
            busy = false
        }
    }
}

struct ContentView: View {
    @EnvironmentObject var model: SampleModel

    var body: some View {
        NavigationView {
            Form {
                Section {
                    Toggle("同意设备与网络信息采集", isOn: $model.consent)
                    Button("更新设备档案") { model.register() }.disabled(!model.consent)
                } footer: { Text("采集机型、系统与运行环境，用于设备登记和风险识别；不需要任何权限。") }
                Section("演示业务") {
                    TextField("账号", text: $model.account)
                    SecureField("密码", text: $model.password)
                    Button("登录") { model.login() }
                    Button("尝试注册") { model.operate("register") }
                    Button("领取演示奖励") { model.operate("reward") }
                    Button("发起演示支付") { model.operate("payment") }
                }
                Section("结果") {
                    Text(model.status).textSelection(.enabled)
                    if model.busy { ProgressView() }
                }
                Section("查看风控结果") {
                    Text("登录工作台 \(Demo.console)\n公开体验账号 \(Demo.consoleAccount)  密码 \(Demo.consolePassword)\n所有体验者共用，数据互相可见").font(.footnote).textSelection(.enabled)
                }
            }
            .navigationTitle("知衡")
        }
        .navigationViewStyle(.stack)
        .onAppear { if model.consent { model.register() } }
    }
}
