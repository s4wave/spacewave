#include "update-manager.h"

#include <cerrno>
#include <cstdio>

#ifdef _WIN32
#include <process.h>
#include <windows.h>
#else
#include <signal.h>
#include <spawn.h>
#include <sys/stat.h>
#include <unistd.h>
#ifdef __APPLE__
#include <sys/event.h>
#else
#include <poll.h>
#include <sys/syscall.h>
#endif
extern char **environ;
#endif

UpdateManager::UpdateManager(const std::string &current_path,
                             const std::string &staged_path, int32_t pid)
    : current_path_(current_path), staged_path_(staged_path), pid_(pid) {}

bool UpdateManager::execute() {
  if (!WaitForProcessExit())
    return false;
  if (!SwapBinary())
    return false;
  if (!Relaunch()) {
    // Retain the failed candidate and restore the last runnable installation.
    if (rename(current_path_.c_str(), staged_path_.c_str()) != 0 ||
        rename((current_path_ + ".old").c_str(), current_path_.c_str()) != 0) {
      fprintf(stderr, "error: failed to restore previous installation\n");
    }
    return false;
  }
  remove((current_path_ + ".old").c_str());
  return true;
}

bool UpdateManager::WaitForProcessExit() {
  if (pid_ <= 0)
    return false;
#ifdef _WIN32
  HANDLE process = OpenProcess(SYNCHRONIZE, FALSE, pid_);
  if (!process)
    return GetLastError() == ERROR_INVALID_PARAMETER;
  const DWORD result = WaitForSingleObject(process, INFINITE);
  CloseHandle(process);
  return result == WAIT_OBJECT_0;
#elif defined(__APPLE__)
  const int queue = kqueue();
  if (queue < 0)
    return false;
  struct kevent event;
  EV_SET(&event, pid_, EVFILT_PROC, EV_ADD | EV_ONESHOT, NOTE_EXIT, 0, nullptr);
  int result = kevent(queue, &event, 1, nullptr, 0, nullptr);
  if (result < 0) {
    const bool exited = errno == ESRCH;
    close(queue);
    return exited;
  }
  do {
    result = kevent(queue, nullptr, 0, &event, 1, nullptr);
  } while (result < 0 && errno == EINTR);
  close(queue);
  return result == 1 && !(event.flags & EV_ERROR);
#else
  const int process = syscall(SYS_pidfd_open, pid_, 0);
  if (process < 0)
    return errno == ESRCH;
  struct pollfd event{process, POLLIN, 0};
  int result;
  do {
    result = poll(&event, 1, -1);
  } while (result < 0 && errno == EINTR);
  close(process);
  return result == 1 && (event.revents & POLLIN);
#endif
}

bool UpdateManager::SwapBinary() {
  std::string backup_path = current_path_ + ".old";

  // Remove old backup if exists.
  remove(backup_path.c_str());

  // Rename current to backup.
  if (rename(current_path_.c_str(), backup_path.c_str()) != 0) {
    fprintf(stderr, "error: failed to rename current to backup\n");
    return false;
  }

  // Move staged into place.
  if (rename(staged_path_.c_str(), current_path_.c_str()) != 0) {
    // Rollback: move backup back.
    rename(backup_path.c_str(), current_path_.c_str());
    fprintf(stderr, "error: failed to move staged into place\n");
    return false;
  }

  // Keep the backup until the replacement has successfully launched.
  return true;
}

bool UpdateManager::Relaunch() {
#ifdef _WIN32
  // Spawn new process and exit.
  STARTUPINFOA si;
  PROCESS_INFORMATION pi;
  memset(&si, 0, sizeof(si));
  si.cb = sizeof(si);
  memset(&pi, 0, sizeof(pi));

  if (!CreateProcessA(current_path_.c_str(), nullptr, nullptr, nullptr, FALSE,
                      0, nullptr, nullptr, &si, &pi)) {
    fprintf(stderr, "error: failed to Relaunch\n");
    return false;
  }
  CloseHandle(pi.hProcess);
  CloseHandle(pi.hThread);
  return true;
#else
  // posix_spawn reports exec errors to the updater, unlike a successful fork.
  if (chmod(current_path_.c_str(), 0755) != 0)
    return false;
  pid_t child;
  std::string executable = current_path_;
  char *args[] = {executable.data(), nullptr};
  const int result = posix_spawn(&child, current_path_.c_str(), nullptr,
                                 nullptr, args, environ);
  if (result != 0) {
    fprintf(stderr, "error: failed to Relaunch (%d)\n", result);
    return false;
  }
  return true;
#endif
}
