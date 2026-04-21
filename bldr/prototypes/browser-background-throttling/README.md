# Background Tab Throttling# Background Tab Throttling Demo

This is a simple HTML page that demonstrates background tab throttling using the
Page Visibility API and `setInterval`. The purpose of this demo is to showcase
how browsers throttle or suspend tabs that are not currently visible to the
user, in order to optimize performance and reduce resource consumption.

## How it works

The demo page uses the Page Visibility API to detect when the page becomes
hidden (i.e., the user switches to a different tab or minimizes the browser
window). It records the timestamp at which the page becomes hidden and logs it
to the console.

A `setInterval` function is used to repeatedly call a `tick` function every
100ms. The `tick` function measures the elapsed time since the last tick and
checks if the tab has become throttled or suspended based on certain thresholds:

- If the elapsed time is greater than or equal to 200ms and the tab is not
  already marked as throttled, it means the tab has been throttled. The
  `throttled` flag is set to `true`, and a message is logged to the console
  indicating the timestamp at which throttling began.

- If the elapsed time is greater than or equal to 2000ms and the tab is not
  already marked as suspended, it means the tab has been suspended. The
  `suspended` flag is set to `true`, and a message is logged to the console
  indicating the timestamp at which suspension occurred.

When the user switches back to the tab, making it visible again, the
`visibilitychange` event is triggered. The demo page resets the `throttled` and
`suspended` flags and logs a message to the console indicating that the page is
visible again.

## Usage

To use this demo:

1. Clone the repository or download the `index.html` file.

2. Open the `index.html` file in a web browser.

3. Open the developer console in the browser to view the throttling events and
   timestamps.

4. Switch to a different tab or minimize the browser window to trigger the "Page
   hidden" event and start the throttling process.

5. Wait for a few seconds and observe the "Tab throttled" and "Tab suspended"
   events and their corresponding timestamps in the console.

6. Switch back to the demo page tab to trigger the "Page visible" event and
   reset the throttling and suspension flags.

## Findings

The Page Visibility API, which is supported in most modern browsers. However,
the actual throttling behavior may vary depending on the browser and its
settings. Some browsers may have different throttling thresholds or may not
throttle background tabs at all.

https://developer.mozilla.org/en-US/docs/Web/API/Page_Visibility_API

  Timer tasks are only permitted when the budget is non-negative. Once a timer's
  code has finished running, the duration of time it took to execute is
  subtracted from its window's timeout budget. The budget regenerates at a rate
  of 10 ms per second, in both Firefox and Chrome.

### Firefox

This demo shows a timer throttling beginning at 1000ms.

From MDN:

  In Firefox, windows in background tabs each have their own time budget in
  milliseconds — a max and a min value of +50 ms and -150 ms, respectively.

The timers are immediately throttled at 1 second, but the actual full throttling
only starts at 30 seconds according to MDN.

### Chrome

This demo shows a timer throttling beginning as soon as 900ms.

From MDN:

  Windows are subjected to throttling after 10 seconds in Chrome.

## Insights

The key insights are:

- Throttling begins almost immediately after backgrounded w/ 1000ms grace period.
- The page can respond to pings when backgrounded but only within the time budget.
- It would be best to shut down any CPU time usage immediately when backgrounded.

## Note: AI authored

Portions of the example in this directory were authored by Claude 3 Opus.
