import DeviceTrust
import SwiftUI

/// The online experience app. Three tabs: the device's risk as the platform
/// sees it, protected demo business operations with their scoring, and how to
/// view the results in the console.
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
        WindowGroup {
            TabView {
                DeviceView().tabItem { Label("设备", systemImage: "iphone") }
                BusinessView().tabItem { Label("业务", systemImage: "creditcard") }
                AboutView().tabItem { Label("说明", systemImage: "info.circle") }
            }
            .tint(.accent)
            .environmentObject(model)
        }
        .onChange(of: phase) { phase in
            if phase == .active { model.refresh() }
        }
    }
}

struct Hit: Identifiable {
    let id = UUID()
    let name: String
    let points: Int

    static func list(_ raw: Any?) -> [Hit] {
        ((raw as? [[String: Any]]) ?? []).compactMap { hit in
            guard let points = hit["points"] as? Int, points != 0 else { return nil }
            let label = hit["label"] as? String ?? ""
            return Hit(name: label.isEmpty ? hit["code"] as? String ?? "" : label, points: points)
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
    // Device tab
    @Published var score: Int?
    @Published var level = ""
    @Published var breakdown = ""
    @Published var summary = ""
    @Published var factors: [Hit] = []
    @Published var status = "准备登记设备"
    // Business tab
    @Published var account = "体验用户"
    @Published var password = Demo.password
    @Published var businessStatus = "登录后选择一项操作"
    @Published var businessScore: Int?
    @Published var businessHits: [Hit] = []
    @Published var busy = false
    private var token = "", platform: URL?, eventId = ""

    func register() {
        run(device: true) {
            let result = try await DeviceTrust.initialize(config: Demo.config, consent: true)
            try await self.loadRisk()
            return "设备已登记 · " + self.proofName(result.proofLevel)
        }
    }

    /// Drops the device key so the next registration enrolls and attests a new one.
    func reenroll() {
        run(device: true) {
            try DeviceTrust.reset(appId: Demo.appId)
            let result = try await DeviceTrust.initialize(config: Demo.config, consent: true)
            try await self.loadRisk()
            return "已重新登记 · " + self.proofName(result.proofLevel)
        }
    }

    func refresh() {
        guard consent else { return }
        run(device: true) {
            guard try await DeviceTrust.refreshIfNeeded(config: Demo.config, consent: true) != nil else {
                try await self.loadRisk()
                return "已刷新"
            }
            try await self.loadRisk()
            return "设备档案已更新"
        }
    }

    func reloadRisk() { run(device: true) { try await self.loadRisk(); return "已刷新" } }

    /// The platform's current view of this device: score, level and factors.
    private func loadRisk() async throws {
        let deviceId = DeviceTrust.deviceId(appId: Demo.appId)
        guard !deviceId.isEmpty else { throw DeviceTrustError.protocolError("设备尚未登记") }
        let v = try await post("device/risk", ["deviceId": deviceId])
        score = v["riskScore"] as? Int
        level = v["riskLevel"] as? String ?? ""
        breakdown = "环境 \(v["envScore"] as? Int ?? 0) + 业务 \(v["businessScore"] as? Int ?? 0)"
        let attestation = (v["app"] as? [String: Any])?["attestation"] as? String ?? ""
        summary = "设备 ID \(deviceId.prefix(16))…\n\(attestation.isEmpty ? "仅签名（无硬件证明）" : "硬件证明通过（App Attest）")\n出口 IP \(v["observedIp"] as? String ?? "")"
        factors = Hit.list(v["factors"])
    }

    func login() {
        run(device: false) {
            let result = try await self.post("login", ["accountId": self.account, "password": self.password])
            self.token = result["token"] as? String ?? ""
            self.platform = (result["platformUrl"] as? String).flatMap(URL.init(string:))
            return "已登录 · \(self.account)"
        }
    }

    func operate(_ kind: String) {
        guard !token.isEmpty, let platform else { businessStatus = "请先登录"; return }
        guard consent else { businessStatus = "请先在设备页同意采集"; return }
        run(device: false) {
            let prepared = try await self.post("prepare", ["type": kind, "keyId": try DeviceTrust.keyId(appId: Demo.appId)])
            guard let eventId = prepared["eventId"] as? String, let challenge = prepared["challenge"] as? [String: Any] else {
                throw DeviceTrustError.protocolError("invalid prepare response")
            }
            self.eventId = eventId
            let report = try await DeviceTrust.report(challenge: challenge, platformURL: platform, pins: Demo.pins)
            let result = try await self.post("complete", ["eventId": eventId, "reportId": report.reportId])
            self.businessScore = result["score"] as? Int
            self.businessHits = Hit.list(result["hits"])
            try await self.loadRisk()
            switch result["state"] as? String {
            case "completed": return "操作已完成 · 请求号 \(eventId.prefix(12))"
            case "verify": return "需要核验 · 请在工作台处理"
            case "hold": return "等待审核 · 请在工作台处理"
            default: throw DeviceTrustError.protocolError("未知业务状态")
            }
        }
    }

    private func proofName(_ level: String) -> String { level == "app_attest" ? "App Attest 硬件证明" : "仅签名" }

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

    private func run(device: Bool, _ work: @escaping () async throws -> String) {
        busy = true
        Task {
            do {
                let message = try await work()
                if device { status = message } else { businessStatus = message }
            } catch {
                if device { status = "失败：\(error)" } else { businessStatus = "失败：\(error)" }
            }
            busy = false
        }
    }
}

extension Color {
    static let accent = Color(red: 20 / 255, green: 125 / 255, blue: 104 / 255)
    static func risk(_ level: String) -> Color { level == "high" ? .red : level == "medium" ? .orange : .accent }
    static func points(_ score: Int) -> Color { score >= 70 ? .red : score >= 40 ? .orange : .accent }
}

struct HitList: View {
    let hits: [Hit]
    let empty: String
    var body: some View {
        if hits.isEmpty {
            Text(empty).foregroundColor(.secondary)
        } else {
            ForEach(hits) { hit in
                HStack {
                    Text(hit.name)
                    Spacer()
                    Text(hit.points > 0 ? "+\(hit.points)" : "\(hit.points)").bold()
                        .foregroundColor(hit.points > 0 ? .red : .accent)
                }
            }
        }
    }
}

struct DeviceView: View {
    @EnvironmentObject var model: SampleModel
    var body: some View {
        NavigationView {
            Form {
                Section {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("设备风险分").font(.subheadline).foregroundColor(.secondary)
                        Text(model.score.map(String.init) ?? "--").font(.system(size: 56, weight: .bold))
                            .foregroundColor(model.score == nil ? .primary : .risk(model.level))
                        Text(model.score == nil ? "尚未登记" : "\(levelName(model.level)) · \(model.breakdown)").foregroundColor(.secondary)
                        Text(model.summary).font(.footnote).foregroundColor(.secondary).textSelection(.enabled)
                    }.padding(.vertical, 6)
                }
                Section("评分条目") { HitList(hits: model.factors, empty: "环境干净，没有命中评分项") }
                Section {
                    Toggle("同意设备与网络信息采集", isOn: $model.consent)
                    Button("更新设备档案") { model.register() }.disabled(!model.consent || model.busy)
                    Button("刷新风险分") { model.reloadRisk() }.disabled(model.busy)
                    Button("重新登记设备") { model.reenroll() }.disabled(!model.consent || model.busy)
                } footer: {
                    Text(model.status).textSelection(.enabled)
                }
            }
            .navigationTitle("知衡 · 风控体验")
        }
        .navigationViewStyle(.stack)
        .onAppear { if model.consent && model.score == nil { model.register() } }
    }

    private func levelName(_ level: String) -> String { level == "high" ? "高风险" : level == "medium" ? "中风险" : "低风险" }
}

struct BusinessView: View {
    @EnvironmentObject var model: SampleModel
    var body: some View {
        NavigationView {
            Form {
                Section("演示业务账号") {
                    TextField("账号", text: $model.account)
                    SecureField("密码", text: $model.password)
                    Button("登录") { model.login() }.disabled(model.busy)
                }
                Section("受保护的操作") {
                    Button("尝试注册") { model.operate("register") }
                    Button("领取演示奖励") { model.operate("reward") }
                    Button("发起演示支付") { model.operate("payment") }
                }.disabled(model.busy)
                Section("本次评估") {
                    Text(model.businessScore.map(String.init) ?? "--").font(.system(size: 40, weight: .bold))
                        .foregroundColor(model.businessScore.map(Color.points) ?? .primary)
                    Text(model.businessStatus).foregroundColor(.secondary).textSelection(.enabled)
                    HitList(hits: model.businessHits, empty: "没有命中评分项")
                }
            }
            .navigationTitle("业务体验")
        }
        .navigationViewStyle(.stack)
    }
}

struct AboutView: View {
    var body: some View {
        NavigationView {
            Form {
                Section("在工作台查看") {
                    Text("网址 \(Demo.console)\n公开体验账号 \(Demo.consoleAccount)\n密码 \(Demo.consolePassword)").textSelection(.enabled)
                    Text("可查看设备档案、事件和评分明细，并处理“需要核验 / 等待审核”的操作。所有体验者共用此账号，数据互相可见。")
                        .font(.footnote).foregroundColor(.secondary)
                }
                Section("怎么体验") {
                    Text("1. 设备页：同意采集后自动登记，显示平台对这台手机的风险分和每一条评分依据。\n2. 开启代理或 VPN、连接外接键盘鼠标、开启辅助触控后点“更新设备档案”，看分数变化。\n3. 业务页：登录后尝试注册、领奖、支付，查看每次操作的评估结果。")
                        .font(.footnote)
                }
                Section { Text("SDK \(DeviceTrust.version)").font(.footnote).foregroundColor(.secondary) }
            }
            .navigationTitle("说明")
        }
        .navigationViewStyle(.stack)
    }
}
