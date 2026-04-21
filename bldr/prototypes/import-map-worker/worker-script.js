// worker-script.js

// Listen for messages from the main script
self.onmessage = function (event) {
  console.log('Received message in worker:', event.data);

  // Send a message back to the main script
  self.postMessage('Hello from the worker!');
};
