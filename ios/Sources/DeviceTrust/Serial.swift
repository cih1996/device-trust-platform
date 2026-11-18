import Foundation

/// Runs SDK operations one at a time, in call order. Concurrent first-launch
/// calls would otherwise each enroll their own device key.
actor Serial {
    static let shared = Serial()
    private var tail: Task<Void, Never>?

    func run<T>(_ operation: @escaping () async throws -> T) async throws -> T {
        let previous = tail
        let task = Task { () async throws -> T in
            _ = await previous?.value
            return try await operation()
        }
        tail = Task { _ = try? await task.value }
        return try await task.value
    }
}
