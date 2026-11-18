#ifndef DEVICE_TRUST_CORE_H
#define DEVICE_TRUST_CORE_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/// 1 when a debugger is attached (P_TRACED), 0 when not, -1 when unknown.
int dtc_debugger_attached(void);

/// LC_ENCRYPTION_INFO cryptid of the main executable: 1 encrypted (App Store),
/// 0 not encrypted (development build or a decrypted dump), -1 no such command.
int dtc_cryptid(void);

/// Jailbreak files found with access(2), newline separated. Returns the count.
int dtc_jailbreak_paths(char *out, size_t size);

/// Candidate jailbreak paths, newline separated, for the Foundation cross-check.
int dtc_jailbreak_candidates(char *out, size_t size);

/// Loaded images outside the system, the app bundle and Xcode tooling,
/// newline separated. Returns the count.
int dtc_foreign_images(char *out, size_t size);

/// Loaded images whose names match hooking or instrumentation tools.
int dtc_hook_images(char *out, size_t size);

/// 1 when a file can be created outside the app sandbox.
int dtc_sandbox_escape(void);

/// 1 when DYLD_INSERT_LIBRARIES is set for this process.
int dtc_dyld_insert(void);

/// 1 when something listens on the Frida default port on loopback.
int dtc_frida_port(void);

#ifdef __cplusplus
}
#endif

#endif
