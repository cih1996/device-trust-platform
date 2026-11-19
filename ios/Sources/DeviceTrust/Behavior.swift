import UIKit

/// Usage statistics for the behaviour profile, matching the Android SDK:
/// sessions, screens, active hours and the "feel" of taps and swipes. Only
/// aggregates since the previous report are kept and uploaded, never
/// coordinates, paths or screen content. Pending statistics are saved when
/// the app enters the background, so a process the system kills loses nothing.
enum Behavior {
    private static let pendingKey = "dtp.behavior.pending"
    private static let lock = NSLock()
    private static var observing = false
    private static var since = Date(), foregroundStart: TimeInterval?
    private static var sessions = 0, screens = 0, foregroundMs: Double = 0
    private static var hours = [Int](repeating: 0, count: 24)
    private static var taps = 0, exactRepeats = 0, intervals = 0
    private static var lastTapAt: TimeInterval = 0, lastTap: CGPoint?
    private static var interval = Stats(), size = Stats(), pressure = Stats()
    private static var cells: [Int: Int] = [:]
    private static var swipes = 0, straightness: Double = 0, speedCv: Double = 0

    /// Starts session counting; called on the main thread by the SDK.
    @MainActor
    static func observeSessions() {
        guard !observing else { return }
        observing = true
        restore()
        let center = NotificationCenter.default
        center.addObserver(forName: UIApplication.didBecomeActiveNotification, object: nil, queue: .main) { _ in begin() }
        center.addObserver(forName: UIApplication.didEnterBackgroundNotification, object: nil, queue: .main) { _ in end() }
        if UIApplication.shared.applicationState == .active { begin() }
    }

    private static func begin() {
        lock.lock(); defer { lock.unlock() }
        guard foregroundStart == nil else { return }
        sessions += 1
        foregroundStart = ProcessInfo.processInfo.systemUptime
        hours[Calendar.current.component(.hour, from: Date())] += 1
    }

    private static func end() {
        lock.lock(); defer { lock.unlock() }
        guard let start = foregroundStart else { return }
        foregroundMs += (ProcessInfo.processInfo.systemUptime - start) * 1000
        foregroundStart = nil
        save()
    }

    /// Counters that outlive the process between reports.
    private struct Pending: Codable {
        var since: Date, sessions: Int, screens: Int, foregroundMs: Double, hours: [Int]
        var taps: Int, exactRepeats: Int, intervals: Int, cells: [Int: Int]
        var interval: Stats, size: Stats, pressure: Stats
        var swipes: Int, straightness: Double, speedCv: Double
    }

    /// Called with the lock held.
    private static func save() {
        let pending = Pending(since: since, sessions: sessions, screens: screens, foregroundMs: foregroundMs, hours: hours, taps: taps,
                              exactRepeats: exactRepeats, intervals: intervals, cells: cells, interval: interval, size: size,
                              pressure: pressure, swipes: swipes, straightness: straightness, speedCv: speedCv)
        UserDefaults.standard.set(try? JSONEncoder().encode(pending), forKey: pendingKey)
    }

    private static func restore() {
        lock.lock(); defer { lock.unlock() }
        guard let data = UserDefaults.standard.data(forKey: pendingKey) else { return }
        UserDefaults.standard.removeObject(forKey: pendingKey)
        guard let p = try? JSONDecoder().decode(Pending.self, from: data) else { return }
        since = min(since, p.since)
        sessions += p.sessions; screens += p.screens; foregroundMs += p.foregroundMs
        for (i, n) in p.hours.prefix(24).enumerated() { hours[i] += n }
        taps += p.taps; exactRepeats += p.exactRepeats; intervals += p.intervals
        cells.merge(p.cells, uniquingKeysWith: +)
        interval.merge(p.interval); size.merge(p.size); pressure.merge(p.pressure)
        swipes += p.swipes; straightness += p.straightness; speedCv += p.speedCv
    }

    static func screen() {
        lock.lock(); defer { lock.unlock() }
        screens += 1
    }

    static func tap(at point: CGPoint, in bounds: CGRect, time: TimeInterval, radius: CGFloat, force: CGFloat) {
        lock.lock(); defer { lock.unlock() }
        taps += 1
        if lastTapAt > 0, time - lastTapAt < 10 { interval.add((time - lastTapAt) * 1000); intervals += 1 }
        if point == lastTap { exactRepeats += 1 }
        lastTapAt = time
        lastTap = point
        size.add(Double(radius))
        pressure.add(Double(force))
        let cx = min(7, max(0, Int(point.x * 8 / max(1, bounds.width))))
        let cy = min(15, max(0, Int(point.y * 16 / max(1, bounds.height))))
        cells[cy * 8 + cx, default: 0] += 1
    }

    static func swipe(straightness s: Double, speedCv cv: Double) {
        lock.lock(); defer { lock.unlock() }
        swipes += 1
        straightness += s
        speedCv += cv
    }

    /// Statistics since the previous snapshot; the counters restart.
    static func snapshot() -> [String: Any] {
        lock.lock(); defer { lock.unlock() }
        let now = Date()
        if let start = foregroundStart {
            let t = ProcessInfo.processInfo.systemUptime
            foregroundMs += (t - start) * 1000
            foregroundStart = t
        }
        let top = cells.values.max() ?? 0
        let out: [String: Any] = [
            "windowMs": Int(now.timeIntervalSince(since) * 1000), "sessions": sessions, "foregroundMs": Int(foregroundMs),
            "screens": screens, "hours": hours,
            "taps": ["count": taps, "intervalMeanMs": round(interval.mean), "intervalCv": round(interval.cv), "intervals": intervals,
                     "cells": cells.count, "topCellShare": taps == 0 ? 0 : round(Double(top) / Double(taps)),
                     "exactRepeat": taps == 0 ? 0 : round(Double(exactRepeats) / Double(taps)),
                     "sizeCv": round(size.cv), "pressureCv": round(pressure.cv)],
            "swipes": ["count": swipes, "straightness": swipes == 0 ? 0 : round(straightness / Double(swipes)),
                       "speedCv": swipes == 0 ? 0 : round(speedCv / Double(swipes))],
        ]
        since = now
        sessions = 0; screens = 0; taps = 0; exactRepeats = 0; intervals = 0; swipes = 0
        foregroundMs = 0; straightness = 0; speedCv = 0
        hours = [Int](repeating: 0, count: 24)
        cells = [:]; interval = Stats(); size = Stats(); pressure = Stats()
        UserDefaults.standard.removeObject(forKey: pendingKey)
        return out
    }

    private static func round(_ v: Double) -> Double { (v * 1000).rounded() / 1000 }

    struct Stats: Codable {
        private var n = 0, sum = 0.0, sq = 0.0
        mutating func add(_ v: Double) { n += 1; sum += v; sq += v * v }
        mutating func merge(_ o: Stats) { n += o.n; sum += o.sum; sq += o.sq }
        var mean: Double { n == 0 ? 0 : sum / Double(n) }
        var cv: Double {
            guard n >= 2, sum != 0 else { return 0 }
            let m = sum / Double(n)
            return sqrt(max(0, sq / Double(n) - m * m)) / m
        }
    }
}

/// Watches touches on a window without taking part in gesture handling: it
/// never recognizes, cancels or delays anything.
final class TouchMonitor: UIGestureRecognizer {
    private var start: (point: CGPoint, time: TimeInterval)?
    private var previous: (point: CGPoint, time: TimeInterval)?
    private var path: Double = 0
    private var speed = Behavior.Stats()
    private var radius: CGFloat = 0, force: CGFloat = 0

    override init(target: Any?, action: Selector?) {
        super.init(target: target, action: action)
        cancelsTouchesInView = false
        delaysTouchesBegan = false
        delaysTouchesEnded = false
    }

    override func canPrevent(_ preventedGestureRecognizer: UIGestureRecognizer) -> Bool { false }
    override func canBePrevented(by preventingGestureRecognizer: UIGestureRecognizer) -> Bool { false }

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent) {
        guard start == nil, let touch = touches.first, let view else { return }
        let p = touch.location(in: view)
        start = (p, touch.timestamp)
        previous = start
        path = 0
        speed = Behavior.Stats()
        radius = touch.majorRadius
        force = touch.force
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent) {
        guard let touch = touches.first, let view, let last = previous else { return }
        let p = touch.location(in: view)
        let step = hypot(p.x - last.point.x, p.y - last.point.y)
        path += step
        if touch.timestamp > last.time { speed.add(step / ((touch.timestamp - last.time) * 1000)) }
        previous = (p, touch.timestamp)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent) {
        defer { reset(); state = .failed }
        guard let touch = touches.first, let view, let begin = start, let last = previous else { return }
        let p = touch.location(in: view)
        path += hypot(p.x - last.point.x, p.y - last.point.y)
        if path < 10, touch.timestamp - begin.time < 0.5 {
            Behavior.tap(at: begin.point, in: view.bounds, time: touch.timestamp, radius: radius, force: force)
        } else if path > 0 {
            Behavior.swipe(straightness: hypot(p.x - begin.point.x, p.y - begin.point.y) / path, speedCv: speed.cv)
        }
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent) {
        reset()
        state = .failed
    }

    override func reset() {
        super.reset()
        start = nil
        previous = nil
    }
}
