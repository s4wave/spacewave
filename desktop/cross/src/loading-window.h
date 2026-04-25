#pragma once
#include "proto.h"
#include <functional>
#include <string>

// Platform-abstracted loading window interface.
class LoadingWindow {
public:
    virtual ~LoadingWindow() = default;

    virtual bool create(const std::string& title, const std::string& iconPath) = 0;
    virtual void show() = 0;
    virtual void handleMessage(const HelperMessage& msg) = 0;
    virtual void runEventLoop() = 0;
    virtual void close() = 0;

    std::function<void()> onRetry;
    std::function<void()> onCancel;
};

// Factory: creates the platform-specific implementation.
LoadingWindow* createLoadingWindow();
