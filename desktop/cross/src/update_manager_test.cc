#include "update-manager.h"

#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <sys/wait.h>
#include <unistd.h>

namespace {
// TestDirectory removes the isolated installation after each rehearsal.
struct TestDirectory {
  std::filesystem::path path;
  ~TestDirectory() {
    std::error_code error;
    std::filesystem::remove_all(path, error);
  }
};

// Read returns the installed bytes for rollback verification.
std::string Read(const std::filesystem::path &path) {
  std::ifstream input(path);
  return {std::istreambuf_iterator<char>(input),
          std::istreambuf_iterator<char>()};
}
} // namespace

int main() {
  char directory[] = "/tmp/spacewave-update-XXXXXX";
  if (!mkdtemp(directory))
    return 1;
  TestDirectory fixture{directory};
  const auto current = fixture.path / "current";
  const auto staged = fixture.path / "staged";

  // Use a completed process so the updater can proceed without timing guesses.
  const pid_t exited = fork();
  if (exited < 0)
    return 1;
  if (exited == 0)
    _exit(0);
  if (waitpid(exited, nullptr, 0) != exited)
    return 1;

  // An executable-format error must restore the installed version byte for
  // byte.
  std::ofstream(current) << "previous version";
  std::ofstream(staged) << "invalid executable";
  UpdateManager rejected(current.string(), staged.string(), exited);
  if (rejected.execute() || Read(current) != "previous version" ||
      Read(staged) != "invalid executable") {
    std::cerr << "failed replacement did not preserve the installed version\n";
    return 1;
  }

  // A missing download also leaves the current installation intact.
  std::filesystem::remove(staged);
  UpdateManager missing(current.string(), staged.string(), exited);
  if (missing.execute() || Read(current) != "previous version")
    return 1;

  // Successful process creation commits the replacement and removes its backup.
  const std::string replacement = "#!/bin/sh\nexit 0\n";
  std::ofstream(staged) << replacement;
  UpdateManager accepted(current.string(), staged.string(), exited);
  if (!accepted.execute() || Read(current) != replacement ||
      std::filesystem::exists(current.string() + ".old"))
    return 1;
  if (waitpid(-1, nullptr, 0) < 0)
    return 1;
  std::cout << "update rollback and replacement checks passed\n";
}
