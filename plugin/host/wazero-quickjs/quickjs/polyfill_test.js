// polyfill_test.js - Tests for QuickJS polyfills
// This file tests the EventTarget, Event, CustomEvent polyfills directly

console.log('Starting polyfill tests...');

// Import the event-target which provides EventTarget, Event, CustomEvent
import { EventTarget, Event, CustomEvent } from './event-target.js';

// Test 1: EventTarget constructor exists
if (typeof EventTarget !== 'function') {
  throw new Error('EventTarget constructor not found');
}
console.log('EventTarget constructor exists');

// Test 2: Event constructor exists
if (typeof Event !== 'function') {
  throw new Error('Event constructor not found');
}
console.log('Event constructor exists');

// Test 3: CustomEvent constructor exists
if (typeof CustomEvent !== 'function') {
  throw new Error('CustomEvent constructor not found');
}
console.log('CustomEvent constructor exists');

// Test 4: Create an EventTarget instance
const target = new EventTarget();
if (!target) {
  throw new Error('Failed to create EventTarget instance');
}
console.log('EventTarget instance created');

// Test 5: Create an Event instance
const event = new Event('test', { bubbles: true, cancelable: true });
if (event.type !== 'test' || !event.bubbles || !event.cancelable) {
  throw new Error('Event properties not set correctly');
}
console.log('Event instance created with correct properties');

// Test 6: addEventListener and dispatchEvent
let eventFired = false;
const listener = (e) => {
  eventFired = true;
  if (e.type !== 'test') {
    throw new Error('Event type mismatch in listener');
  }
};
target.addEventListener('test', listener);
target.dispatchEvent(event);
if (!eventFired) {
  throw new Error('Event listener was not called');
}
console.log('addEventListener and dispatchEvent work');

// Test 7: removeEventListener
eventFired = false;
target.removeEventListener('test', listener);
target.dispatchEvent(new Event('test'));
if (eventFired) {
  throw new Error('Event listener was not removed');
}
console.log('removeEventListener works');

// Test 8: CustomEvent with detail
const customEvent = new CustomEvent('custom', { detail: { foo: 'bar' } });
if (!customEvent.detail || customEvent.detail.foo !== 'bar') {
  throw new Error('CustomEvent detail not set correctly');
}
console.log('CustomEvent with detail works');

// Test 9: preventDefault
const cancelableEvent = new Event('cancel', { cancelable: true });
cancelableEvent.preventDefault();
if (!cancelableEvent.defaultPrevented) {
  throw new Error('preventDefault did not set defaultPrevented');
}
console.log('preventDefault works');

// Test 10: stopPropagation
let firstListenerCalled = false;
let secondListenerCalled = false;
const target2 = new EventTarget();
target2.addEventListener('stop', (e) => {
  firstListenerCalled = true;
  e.stopImmediatePropagation();
});
target2.addEventListener('stop', () => {
  secondListenerCalled = true;
});
target2.dispatchEvent(new Event('stop'));
if (!firstListenerCalled || secondListenerCalled) {
  throw new Error('stopImmediatePropagation did not work');
}
console.log('stopImmediatePropagation works');

// Test 11: Event once option
let onceCallCount = 0;
const target3 = new EventTarget();
target3.addEventListener('once', () => {
  onceCallCount++;
}, { once: true });
target3.dispatchEvent(new Event('once'));
target3.dispatchEvent(new Event('once'));
if (onceCallCount !== 1) {
  throw new Error('once option did not work, called ' + onceCallCount + ' times');
}
console.log('addEventListener with once option works');

// Test 12: Event phases
const ev = new Event('phase');
if (ev.NONE !== 0 || ev.CAPTURING_PHASE !== 1 || ev.AT_TARGET !== 2 || ev.BUBBLING_PHASE !== 3) {
  throw new Error('Event phase constants not correct');
}
console.log('Event phase constants work');

// Test 13: composedPath
const target4 = new EventTarget();
const event4 = new Event('composed');
target4.addEventListener('composed', (e) => {
  const path = e.composedPath();
  if (!Array.isArray(path) || path.length !== 1 || path[0] !== target4) {
    throw new Error('composedPath did not return correct path');
  }
});
target4.dispatchEvent(event4);
console.log('composedPath works');

// Test 14: Test specialized event types
import { CloseEvent, ErrorEvent, MessageEvent, ProgressEvent, PromiseRejectionEvent } from './event-target-polyfill.js';

// Test CloseEvent
const closeEvent = new CloseEvent('close', { code: 1000, reason: 'Normal', wasClean: true });
if (closeEvent.code !== 1000 || closeEvent.reason !== 'Normal' || !closeEvent.wasClean) {
  throw new Error('CloseEvent properties not correct');
}
console.log('CloseEvent works');

// Test ErrorEvent
const error = new Error('Test error');
const errorEvent = new ErrorEvent(error);
if (errorEvent.error !== error || errorEvent.type !== 'error') {
  throw new Error('ErrorEvent properties not correct');
}
console.log('ErrorEvent works');

// Test MessageEvent
const messageEvent = new MessageEvent('message', { test: 'data' });
if (messageEvent.data.test !== 'data' || messageEvent.type !== 'message') {
  throw new Error('MessageEvent properties not correct');
}
console.log('MessageEvent works');

// Test ProgressEvent
const progressEvent = new ProgressEvent('progress', { lengthComputable: true, loaded: 50, total: 100 });
if (!progressEvent.lengthComputable || progressEvent.loaded !== 50 || progressEvent.total !== 100) {
  throw new Error('ProgressEvent properties not correct');
}
console.log('ProgressEvent works');

// Test PromiseRejectionEvent
const promise = Promise.reject('test');
promise.catch(() => {}); // Catch to avoid unhandled rejection
const promiseEvent = new PromiseRejectionEvent('unhandledrejection', promise, 'test');
if (promiseEvent.promise !== promise || promiseEvent.reason !== 'test') {
  throw new Error('PromiseRejectionEvent properties not correct');
}
console.log('PromiseRejectionEvent works');

console.log('\nAll EventTarget polyfill tests passed!');
