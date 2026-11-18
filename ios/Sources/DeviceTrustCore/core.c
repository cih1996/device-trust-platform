#include "DeviceTrustCore.h"

#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <mach-o/dyld.h>
#include <mach-o/loader.h>
#include <netinet/in.h>
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <sys/socket.h>
#include <sys/sysctl.h>
#include <unistd.h>

// Files that only exist on jailbroken devices: package managers, Substrate and
// its successors, rootless (/var/jb) and rootful bootstraps, and SSH.
static const char *const JAILBREAK_PATHS[] = {
    "/Applications/Cydia.app",
    "/Applications/Sileo.app",
    "/Applications/Zebra.app",
    "/Applications/Filza.app",
    "/Applications/TrollStore.app",
    "/Library/MobileSubstrate/MobileSubstrate.dylib",
    "/Library/MobileSubstrate/DynamicLibraries",
    "/usr/lib/TweakInject",
    "/usr/lib/libsubstrate.dylib",
    "/usr/lib/libhooker.dylib",
    "/usr/lib/substitute-inserter.dylib",
    "/var/jb",
    "/var/binpack",
    "/.installed_dopamine",
    "/.installed_palera1n",
    "/.bootstrapped",
    "/etc/apt",
    "/private/var/lib/apt",
    "/private/var/lib/cydia",
    "/usr/sbin/sshd",
    "/usr/bin/ssh",
    "/bin/bash",
    "/usr/libexec/cydia",
};

// Lower-case name fragments of hooking and instrumentation libraries.
static const char *const HOOK_NAMES[] = {
    "frida", "gadget", "substrate", "substitute", "tweakinject", "libhooker",
    "ellekit", "sslkillswitch", "cycript", "shadow.dylib", "choicy", "liberty",
    "flyjb", "hidejb", "revealserver", "libinjector",
};

// Images loaded from these prefixes belong to the OS or Xcode tooling.
static const char *const SYSTEM_PREFIXES[] = {
    "/System/", "/usr/lib/", "/usr/libexec/", "/private/preboot/Cryptexes/", "/Developer/",
};

#define COUNT(a) (sizeof(a) / sizeof((a)[0]))

// append writes one line to out; it never writes past size and keeps out terminated.
static void append(char *out, size_t size, size_t *used, const char *line) {
    if (out == NULL || size == 0) return;
    size_t length = strlen(line);
    if (*used + length + 2 > size) return;
    if (*used > 0) out[(*used)++] = '\n';
    memcpy(out + *used, line, length);
    *used += length;
    out[*used] = '\0';
}

static void start(char *out, size_t size, size_t *used) {
    *used = 0;
    if (out != NULL && size > 0) out[0] = '\0';
}

int dtc_debugger_attached(void) {
    struct kinfo_proc info;
    size_t size = sizeof(info);
    int mib[4] = {CTL_KERN, KERN_PROC, KERN_PROC_PID, getpid()};
    memset(&info, 0, sizeof(info));
    if (sysctl(mib, 4, &info, &size, NULL, 0) != 0) return -1;
    return (info.kp_proc.p_flag & P_TRACED) != 0;
}

int dtc_cryptid(void) {
    const struct mach_header *header = _dyld_get_image_header(0);
    if (header == NULL || header->magic != MH_MAGIC_64) return -1;
    const uint8_t *cursor = (const uint8_t *)header + sizeof(struct mach_header_64);
    for (uint32_t i = 0; i < header->ncmds; i++) {
        const struct load_command *command = (const struct load_command *)cursor;
        if (command->cmd == LC_ENCRYPTION_INFO_64) {
            return ((const struct encryption_info_command_64 *)command)->cryptid != 0;
        }
        cursor += command->cmdsize;
    }
    return -1;
}

int dtc_jailbreak_paths(char *out, size_t size) {
    size_t used;
    int found = 0;
    start(out, size, &used);
    for (size_t i = 0; i < COUNT(JAILBREAK_PATHS); i++) {
        // access(2), not stat(2): file timestamp APIs need a privacy-manifest reason.
        if (access(JAILBREAK_PATHS[i], F_OK) == 0) {
            append(out, size, &used, JAILBREAK_PATHS[i]);
            found++;
        }
    }
    return found;
}

int dtc_jailbreak_candidates(char *out, size_t size) {
    size_t used;
    start(out, size, &used);
    for (size_t i = 0; i < COUNT(JAILBREAK_PATHS); i++) append(out, size, &used, JAILBREAK_PATHS[i]);
    return (int)COUNT(JAILBREAK_PATHS);
}

// bundleDirectory is the .app directory of the main executable.
static size_t bundleDirectory(char *dir, size_t size) {
    const char *main = _dyld_get_image_name(0);
    if (main == NULL) return 0;
    const char *slash = strrchr(main, '/');
    if (slash == NULL || (size_t)(slash - main) + 2 > size) return 0;
    size_t length = (size_t)(slash - main) + 1;
    memcpy(dir, main, length);
    dir[length] = '\0';
    return length;
}

static int hasPrefix(const char *s, const char *prefix) { return strncmp(s, prefix, strlen(prefix)) == 0; }

int dtc_foreign_images(char *out, size_t size) {
    char bundle[1024];
    size_t bundleLength = bundleDirectory(bundle, sizeof(bundle));
    size_t used;
    int found = 0;
    start(out, size, &used);
    uint32_t count = _dyld_image_count();
    for (uint32_t i = 0; i < count; i++) {
        const char *name = _dyld_get_image_name(i);
        if (name == NULL) continue;
        int known = bundleLength > 0 && strncmp(name, bundle, bundleLength) == 0;
        for (size_t p = 0; !known && p < COUNT(SYSTEM_PREFIXES); p++) known = hasPrefix(name, SYSTEM_PREFIXES[p]);
        if (!known) {
            append(out, size, &used, name);
            found++;
        }
    }
    return found;
}

int dtc_hook_images(char *out, size_t size) {
    size_t used;
    int found = 0;
    start(out, size, &used);
    uint32_t count = _dyld_image_count();
    char lower[1024];
    for (uint32_t i = 0; i < count; i++) {
        const char *name = _dyld_get_image_name(i);
        if (name == NULL) continue;
        size_t n = strlen(name);
        if (n >= sizeof(lower)) n = sizeof(lower) - 1;
        for (size_t c = 0; c < n; c++) lower[c] = (char)((name[c] >= 'A' && name[c] <= 'Z') ? name[c] + 32 : name[c]);
        lower[n] = '\0';
        for (size_t h = 0; h < COUNT(HOOK_NAMES); h++) {
            if (strstr(lower, HOOK_NAMES[h]) != NULL) {
                append(out, size, &used, name);
                found++;
                break;
            }
        }
    }
    return found;
}

int dtc_sandbox_escape(void) {
    static const char *const probe = "/private/dtp-sandbox-probe";
    int fd = open(probe, O_CREAT | O_WRONLY | O_EXCL, 0600);
    if (fd < 0) return 0;
    close(fd);
    unlink(probe);
    return 1;
}

int dtc_dyld_insert(void) {
    const char *value = getenv("DYLD_INSERT_LIBRARIES");
    return value != NULL && value[0] != '\0';
}

int dtc_frida_port(void) {
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd < 0) return 0;
    fcntl(fd, F_SETFL, fcntl(fd, F_GETFL, 0) | O_NONBLOCK);
    struct sockaddr_in address;
    memset(&address, 0, sizeof(address));
    address.sin_len = sizeof(address);
    address.sin_family = AF_INET;
    address.sin_port = htons(27042);
    address.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    int open = 0;
    if (connect(fd, (struct sockaddr *)&address, sizeof(address)) == 0) {
        open = 1;
    } else if (errno == EINPROGRESS) {
        struct pollfd p = {fd, POLLOUT, 0};
        if (poll(&p, 1, 150) == 1) {
            int error = 0;
            socklen_t length = sizeof(error);
            open = getsockopt(fd, SOL_SOCKET, SO_ERROR, &error, &length) == 0 && error == 0;
        }
    }
    close(fd);
    return open;
}
