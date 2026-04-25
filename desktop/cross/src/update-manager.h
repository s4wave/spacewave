#pragma once
#include <cstdint>
#include <string>

// UpdateManager handles entrypoint self-update: wait for PID exit,
// swap binary, relaunch.
class UpdateManager {
public:
    UpdateManager(const std::string& currentPath,
                  const std::string& stagedPath,
                  int32_t pid);

    bool execute();

private:
    void waitForProcessExit();
    bool swapBinary();
    bool relaunch();

    std::string currentPath_;
    std::string stagedPath_;
    int32_t pid_;
};
