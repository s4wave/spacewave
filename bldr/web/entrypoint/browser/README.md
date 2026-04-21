# Web Browser

The browser target implements deploying an app to a web page.

The user navigates to the app, which loads the Go runtime in a WebWorker. The
root WebView cannot be closed (as per browser policy). A service worker is used
to intercept requests and forward them to the Go runtime for processing. The
root page can create and close more pages through the WebView API.
