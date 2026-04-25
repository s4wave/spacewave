#include "../../src/loading-window.h"
#include <X11/Xlib.h>
#include <X11/Xutil.h>
// Xlib defines Status as a preprocessor macro; undef so MessageType::Status
// resolves as an identifier.
#undef Status
#include <cstdio>
#include <cstring>

// Minimal X11 loading window. No GTK dependency.
class X11LoadingWindow : public LoadingWindow {
public:
    ~X11LoadingWindow() override { close(); }

    bool create(const std::string& title, const std::string& iconPath) override {
        display_ = XOpenDisplay(nullptr);
        if (!display_) return false;

        int screen = DefaultScreen(display_);
        window_ = XCreateSimpleWindow(display_, RootWindow(display_, screen),
                                      0, 0, 400, 300, 1,
                                      BlackPixel(display_, screen),
                                      WhitePixel(display_, screen));

        XStoreName(display_, window_, title.c_str());

        // Handle window close.
        wmDeleteMessage_ = XInternAtom(display_, "WM_DELETE_WINDOW", False);
        XSetWMProtocols(display_, window_, &wmDeleteMessage_, 1);

        gc_ = XCreateGC(display_, window_, 0, nullptr);
        return true;
    }

    void show() override {
        XMapWindow(display_, window_);

        // Center on screen.
        int screen = DefaultScreen(display_);
        int sw = DisplayWidth(display_, screen);
        int sh = DisplayHeight(display_, screen);
        XMoveWindow(display_, window_, (sw - 400) / 2, (sh - 300) / 2);

        XFlush(display_);
    }

    void handleMessage(const HelperMessage& msg) override {
        switch (msg.type) {
        case MessageType::Progress:
            fraction_ = msg.progress.fraction;
            statusText_ = msg.progress.text;
            redraw();
            break;
        case MessageType::Status:
            statusText_ = msg.status.text;
            redraw();
            break;
        case MessageType::Dismiss:
            running_ = false;
            break;
        case MessageType::Error:
            statusText_ = msg.error.message;
            isError_ = true;
            retryable_ = msg.error.retryable;
            redraw();
            break;
        default:
            break;
        }
    }

    void runEventLoop() override {
        running_ = true;
        redraw();
        while (running_) {
            while (XPending(display_) > 0) {
                XEvent event;
                XNextEvent(display_, &event);
                if (event.type == Expose) {
                    redraw();
                } else if (event.type == ClientMessage) {
                    if ((Atom)event.xclient.data.l[0] == wmDeleteMessage_) {
                        if (onCancel) onCancel();
                    }
                } else if (event.type == ButtonPress) {
                    // Check if retry button area was clicked.
                    if (retryable_ && event.xbutton.x >= 150 && event.xbutton.x <= 250
                        && event.xbutton.y >= 230 && event.xbutton.y <= 260) {
                        isError_ = false;
                        retryable_ = false;
                        statusText_ = "Retrying...";
                        fraction_ = -1;
                        redraw();
                        if (onRetry) onRetry();
                    }
                }
            }
            // Brief sleep to avoid busy-waiting.
            struct timespec ts = {0, 16000000}; // ~16ms
            nanosleep(&ts, nullptr);
        }
    }

    void close() override {
        if (display_) {
            if (gc_) XFreeGC(display_, gc_);
            XDestroyWindow(display_, window_);
            XCloseDisplay(display_);
            display_ = nullptr;
        }
    }

private:
    void redraw() {
        if (!display_) return;
        int screen = DefaultScreen(display_);

        // Clear.
        XSetForeground(display_, gc_, WhitePixel(display_, screen));
        XFillRectangle(display_, window_, gc_, 0, 0, 400, 300);

        // Status text.
        XSetForeground(display_, gc_, isError_ ? 0xCC0000 : BlackPixel(display_, screen));
        if (!statusText_.empty()) {
            XDrawString(display_, window_, gc_,
                        200 - int(statusText_.size() * 3), 180,
                        statusText_.c_str(), int(statusText_.size()));
        }

        // Progress bar background.
        XSetForeground(display_, gc_, 0xDDDDDD);
        XFillRectangle(display_, window_, gc_, 50, 195, 300, 20);

        // Progress bar fill.
        if (fraction_ >= 0) {
            XSetForeground(display_, gc_, 0x4488FF);
            int width = int(fraction_ * 300);
            if (width > 300) width = 300;
            XFillRectangle(display_, window_, gc_, 50, 195, width, 20);
        } else {
            // Indeterminate: pulsing bar.
            XSetForeground(display_, gc_, 0x4488FF);
            XFillRectangle(display_, window_, gc_, 50, 195, 100, 20);
        }

        // Retry button.
        if (retryable_) {
            XSetForeground(display_, gc_, 0xEEEEEE);
            XFillRectangle(display_, window_, gc_, 150, 230, 100, 30);
            XSetForeground(display_, gc_, BlackPixel(display_, screen));
            XDrawRectangle(display_, window_, gc_, 150, 230, 100, 30);
            XDrawString(display_, window_, gc_, 183, 250, "Retry", 5);
        }

        XFlush(display_);
    }

    Display* display_ = nullptr;
    Window window_ = 0;
    GC gc_ = nullptr;
    Atom wmDeleteMessage_ = 0;
    bool running_ = false;
    float fraction_ = -1;
    std::string statusText_ = "Connecting...";
    bool isError_ = false;
    bool retryable_ = false;
};

LoadingWindow* createLoadingWindow() {
    return new X11LoadingWindow();
}
