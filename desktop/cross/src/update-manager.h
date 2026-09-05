#pragma once
#include <cstdint>
#include <string>

// UpdateManager handles entrypoint self-update: wait for PID exit,
// swap binary, Relaunch.
class UpdateManager {
public:
  // UpdateManager retains the installed and staged paths and the exiting app
  // PID.
  UpdateManager(const std::string &current_path, const std::string &staged_path,
                int32_t pid);

  // execute retains the previous binary until the replacement process launches.
  bool execute();

private:
  // WaitForProcessExit waits for an operating-system process-exit event.
  bool WaitForProcessExit();
  // SwapBinary retains the installed binary as a rollback candidate.
  bool SwapBinary();
  // Relaunch succeeds only when the operating system creates the replacement
  // process.
  bool Relaunch();

  // current_path_ names the installed entrypoint.
  std::string current_path_;
  // staged_path_ names the verified download on the same filesystem.
  std::string staged_path_;
  // pid_ identifies the application that must exit before replacement.
  int32_t pid_;
};
