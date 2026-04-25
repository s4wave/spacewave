#include "update-manager.h"
#include <cstdio>

#ifdef _WIN32
#include <windows.h>
#include <process.h>
#else
#include <signal.h>
#include <unistd.h>
#include <sys/stat.h>
#endif

UpdateManager::UpdateManager(const std::string& currentPath,
                             const std::string& stagedPath,
                             int32_t pid)
    : currentPath_(currentPath), stagedPath_(stagedPath), pid_(pid) {}

bool UpdateManager::execute() {
    waitForProcessExit();
    if (!swapBinary()) return false;
    return relaunch();
}

void UpdateManager::waitForProcessExit() {
#ifdef _WIN32
    HANDLE hProcess = OpenProcess(SYNCHRONIZE, FALSE, pid_);
    if (hProcess) {
        WaitForSingleObject(hProcess, 30000); // 30s timeout
        CloseHandle(hProcess);
    }
#else
    // Poll for process exit.
    while (kill(pid_, 0) == 0) {
        usleep(100000); // 100ms
    }
#endif
}

bool UpdateManager::swapBinary() {
    std::string backupPath = currentPath_ + ".old";

    // Remove old backup if exists.
    remove(backupPath.c_str());

    // Rename current to backup.
    if (rename(currentPath_.c_str(), backupPath.c_str()) != 0) {
        fprintf(stderr, "error: failed to rename current to backup\n");
        return false;
    }

    // Move staged into place.
    if (rename(stagedPath_.c_str(), currentPath_.c_str()) != 0) {
        // Rollback: move backup back.
        rename(backupPath.c_str(), currentPath_.c_str());
        fprintf(stderr, "error: failed to move staged into place\n");
        return false;
    }

    // Remove backup.
    remove(backupPath.c_str());
    return true;
}

bool UpdateManager::relaunch() {
#ifdef _WIN32
    // Spawn new process and exit.
    STARTUPINFOA si;
    PROCESS_INFORMATION pi;
    memset(&si, 0, sizeof(si));
    si.cb = sizeof(si);
    memset(&pi, 0, sizeof(pi));

    if (!CreateProcessA(currentPath_.c_str(), nullptr, nullptr, nullptr,
                        FALSE, 0, nullptr, nullptr, &si, &pi)) {
        fprintf(stderr, "error: failed to relaunch\n");
        return false;
    }
    CloseHandle(pi.hProcess);
    CloseHandle(pi.hThread);
    return true;
#else
    // Fork and exec.
    pid_t child = fork();
    if (child == 0) {
        // Set executable permission.
        chmod(currentPath_.c_str(), 0755);
        execl(currentPath_.c_str(), currentPath_.c_str(), nullptr);
        _exit(1);
    }
    return child > 0;
#endif
}
