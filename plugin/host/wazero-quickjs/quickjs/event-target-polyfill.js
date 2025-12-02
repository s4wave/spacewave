/**
 * @source https://github.com/nicholascc/txiki.js/blob/e359e5/src/js/polyfills/event-target-polyfill.js
 *
 * Specialized event types built on top of Event/EventTarget.
 */

import { Event } from './event-target.js';

const kCloseEventCode = Symbol('kCloseEventCode');
const kCloseEventReason = Symbol('kCloseEventReason');
const kCloseEventWasClean = Symbol('kCloseEventWasClean');

class CloseEvent extends Event {
    constructor(eventType, init) {
        super(eventType, init);

        this[kCloseEventCode] = init?.code ?? 0;
        this[kCloseEventReason] = init?.reason ?? '';
        this[kCloseEventWasClean] = init?.wasClean ?? false;
    }

    get code() {
        return this[kCloseEventCode];
    }

    get reason() {
        return this[kCloseEventReason];
    }

    get wasClean() {
        return this[kCloseEventWasClean];
    }
}

const kErrorEventData = Symbol('kErrorEventData');

class ErrorEvent extends Event {
    constructor(error) {
        super('error');

        this[kErrorEventData] = error;
    }

    get message() {
        return String(this[kErrorEventData]);
    }

    get filename() {
        return undefined;
    }

    get lineno() {
        return undefined;
    }

    get colno() {
        return undefined;
    }

    get error() {
        return this[kErrorEventData];
    }
}

const kMessageEventData = Symbol('kMessageEventData');

class MessageEvent extends Event {
    constructor(eventType, data) {
        super(eventType);

        this[kMessageEventData] = data;
    }

    get data() {
        return this[kMessageEventData];
    }
}

const kPromise = Symbol('kPromise');
const kPromiseRejectionReason = Symbol('kPromiseRejectionReason');

class PromiseRejectionEvent extends Event {
    constructor(eventType, promise, reason) {
        super(eventType, { cancelable: true });

        this[kPromise] = promise;
        this[kPromiseRejectionReason] = reason;
    }

    get promise() {
        return this[kPromise];
    }

    get reason() {
        return this[kPromiseRejectionReason];
    }
}

const kProgressEventLengthComputable = Symbol('kProgressEventLengthComputable');
const kProgressEventLoaded = Symbol('kProgressEventLoaded');
const kProgressEventTotal = Symbol('kProgressEventTotal');

class ProgressEvent extends Event {
    constructor(eventType, init) {
        super(eventType, init);

        this[kProgressEventLengthComputable] = init?.lengthComputable || false;
        this[kProgressEventLoaded] = init?.loaded || 0;
        this[kProgressEventTotal] = init?.total || 0;
    }

    get lengthComputable() {
        return this[kProgressEventLengthComputable];
    }

    get loaded() {
        return this[kProgressEventLoaded];
    }

    get total() {
        return this[kProgressEventTotal];
    }
}

export { CloseEvent, ErrorEvent, MessageEvent, PromiseRejectionEvent, ProgressEvent };
