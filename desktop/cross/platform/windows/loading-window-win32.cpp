#include "../../src/loading-window.h"
#include <windows.h>
#include <commctrl.h>
#include <cstdio>
#include <string>

#pragma comment(lib, "comctl32.lib")

class Win32LoadingWindow;
static Win32LoadingWindow* g_instance = nullptr;

// Win32 loading window using CreateWindowEx and progress bar common control.
class Win32LoadingWindow : public LoadingWindow {
public:
    ~Win32LoadingWindow() override { close(); }

    bool create(const std::string& title, const std::string& iconPath) override {
        g_instance = this;

        INITCOMMONCONTROLSEX icc;
        icc.dwSize = sizeof(icc);
        icc.dwICC = ICC_PROGRESS_CLASS;
        InitCommonControlsEx(&icc);

        WNDCLASSEXA wc = {};
        wc.cbSize = sizeof(wc);
        wc.lpfnWndProc = WndProc;
        wc.hInstance = GetModuleHandle(nullptr);
        wc.hCursor = LoadCursor(nullptr, IDC_ARROW);
        wc.hbrBackground = (HBRUSH)(COLOR_WINDOW + 1);
        wc.lpszClassName = "SpacewaveHelper";
        RegisterClassExA(&wc);

        hwnd_ = CreateWindowExA(0, "SpacewaveHelper", title.c_str(),
                                WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU,
                                CW_USEDEFAULT, CW_USEDEFAULT, 420, 330,
                                nullptr, nullptr, GetModuleHandle(nullptr), nullptr);
        if (!hwnd_) return false;

        // Progress bar.
        progressBar_ = CreateWindowExA(0, PROGRESS_CLASSA, nullptr,
                                       WS_CHILD | WS_VISIBLE | PBS_SMOOTH,
                                       50, 195, 300, 20,
                                       hwnd_, nullptr, GetModuleHandle(nullptr), nullptr);
        SendMessage(progressBar_, PBM_SETRANGE32, 0, 1000);
        SendMessage(progressBar_, PBM_SETMARQUEE, TRUE, 30);
        SetWindowLong(progressBar_, GWL_STYLE,
                      GetWindowLong(progressBar_, GWL_STYLE) | PBS_MARQUEE);

        // Status label.
        statusLabel_ = CreateWindowExA(0, "STATIC", "Connecting...",
                                       WS_CHILD | WS_VISIBLE | SS_CENTER,
                                       50, 170, 300, 20,
                                       hwnd_, nullptr, GetModuleHandle(nullptr), nullptr);

        // Retry button (hidden).
        retryButton_ = CreateWindowExA(0, "BUTTON", "Retry",
                                       WS_CHILD | BS_PUSHBUTTON,
                                       150, 230, 100, 30,
                                       hwnd_, (HMENU)1001, GetModuleHandle(nullptr), nullptr);

        return true;
    }

    void show() override {
        ShowWindow(hwnd_, SW_SHOW);
        UpdateWindow(hwnd_);

        // Center on screen.
        RECT rc;
        GetWindowRect(hwnd_, &rc);
        int w = rc.right - rc.left;
        int h = rc.bottom - rc.top;
        int sw = GetSystemMetrics(SM_CXSCREEN);
        int sh = GetSystemMetrics(SM_CYSCREEN);
        SetWindowPos(hwnd_, nullptr, (sw - w) / 2, (sh - h) / 2, 0, 0,
                     SWP_NOSIZE | SWP_NOZORDER);
    }

    void handleMessage(const HelperMessage& msg) override {
        switch (msg.type) {
        case MessageType::Progress:
            isError_ = false;
            ShowWindow(retryButton_, SW_HIDE);
            if (msg.progress.fraction < 0) {
                SetWindowLong(progressBar_, GWL_STYLE,
                              GetWindowLong(progressBar_, GWL_STYLE) | PBS_MARQUEE);
                SendMessage(progressBar_, PBM_SETMARQUEE, TRUE, 30);
            } else {
                SetWindowLong(progressBar_, GWL_STYLE,
                              GetWindowLong(progressBar_, GWL_STYLE) & ~PBS_MARQUEE);
                SendMessage(progressBar_, PBM_SETPOS, int(msg.progress.fraction * 1000), 0);
            }
            if (!msg.progress.text.empty()) {
                SetWindowTextA(statusLabel_, msg.progress.text.c_str());
            }
            break;
        case MessageType::Status:
            SetWindowTextA(statusLabel_, msg.status.text.c_str());
            break;
        case MessageType::Dismiss:
            PostQuitMessage(0);
            break;
        case MessageType::Error:
            isError_ = true;
            SetWindowTextA(statusLabel_, msg.error.message.c_str());
            if (msg.error.retryable) {
                ShowWindow(retryButton_, SW_SHOW);
            }
            break;
        default:
            break;
        }
    }

    void runEventLoop() override {
        MSG msg;
        while (GetMessage(&msg, nullptr, 0, 0)) {
            TranslateMessage(&msg);
            DispatchMessage(&msg);
        }
    }

    void close() override {
        if (hwnd_) {
            DestroyWindow(hwnd_);
            hwnd_ = nullptr;
        }
    }

private:
    static LRESULT CALLBACK WndProc(HWND hwnd, UINT msg, WPARAM wParam, LPARAM lParam) {
        switch (msg) {
        case WM_CLOSE:
            if (g_instance && g_instance->onCancel) g_instance->onCancel();
            return 0;
        case WM_COMMAND:
            if (LOWORD(wParam) == 1001 && g_instance) {
                ShowWindow(g_instance->retryButton_, SW_HIDE);
                SetWindowTextA(g_instance->statusLabel_, "Retrying...");
                SetWindowLong(g_instance->progressBar_, GWL_STYLE,
                              GetWindowLong(g_instance->progressBar_, GWL_STYLE) | PBS_MARQUEE);
                SendMessage(g_instance->progressBar_, PBM_SETMARQUEE, TRUE, 30);
                if (g_instance->onRetry) g_instance->onRetry();
            }
            return 0;
        case WM_DESTROY:
            PostQuitMessage(0);
            return 0;
        }
        return DefWindowProc(hwnd, msg, wParam, lParam);
    }

    HWND hwnd_ = nullptr;
    HWND progressBar_ = nullptr;
    HWND statusLabel_ = nullptr;
    HWND retryButton_ = nullptr;
    bool isError_ = false;
};

LoadingWindow* createLoadingWindow() {
    return new Win32LoadingWindow();
}
