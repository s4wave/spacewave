// https://www.w3.org/TR/user-timing/
// Derived from: https://github.com/blackswanny/performance-polyfill by RubaXa <trash@rubaxa.org>

// This file is governed by txiki.js MIT license.
// See: https://github.com/saghul/txiki.js/blob/6e01bf4/src/js/polyfills/performance.js

let entries = [];
const marksIndex = Object.create(null);

function mark(name) {
  const mark = {
    name,
    entryType: 'mark',
    startTime: globalThis.performance.now(),
    duration: 0,
  };

  entries.push(mark);
  marksIndex[name] = mark;

  return mark;
}

function measure(name, startMark, endMark) {
  let startTime;
  let endTime;

  if (endMark !== undefined && marksIndex[endMark] === undefined) {
    throw new SyntaxError(
      "Failed to execute 'measure' on 'Performance': The mark '" +
        endMark +
        "' does not exist.",
    );
  }

  if (startMark !== undefined && marksIndex[startMark] === undefined) {
    throw new SyntaxError(
      "Failed to execute 'measure' on 'Performance': The mark '" +
        startMark +
        "' does not exist.",
    );
  }

  if (marksIndex[startMark]) {
    startTime = marksIndex[startMark].startTime;
  } else {
    startTime = 0;
  }

  if (marksIndex[endMark]) {
    endTime = marksIndex[endMark].startTime;
  } else {
    endTime = globalThis.performance.now();
  }

  const mark = {
    name,
    entryType: 'measure',
    startTime,
    duration: endTime - startTime,
  };

  entries.push(mark);

  return mark;
}

function getEntriesByType(type) {
  return entries.filter((entry) => entry.entryType === type);
}

function getEntriesByName(name) {
  return entries.filter((entry) => entry.name === name);
}

function clearMarks(name) {
  if (typeof name === 'undefined') {
    entries = entries.filter((entry) => entry.entryType !== 'mark');
  } else {
    const entry = entries.find(
      (e) => e.entryType === 'mark' && e.name === name,
    );

    entries.splice(entries.indexOf(entry), 1);
    delete marksIndex[name];
  }
}

function clearMeasures(name) {
  if (typeof name === 'undefined') {
    entries = entries.filter((entry) => entry.entryType !== 'measure');
  } else {
    const entry = entries.find(
      (e) => e.entryType === 'measure' && e.name === name,
    );

    entries.splice(entries.indexOf(entry), 1);
  }
}

// createQuickjsPerformance creates a performance instance optimized for QuickJS environment.
function createQuickjsPerformance(originalPerformance) {
  // Extend the original performance object with additional methods
  const enhancedPerformance = Object.create(originalPerformance);

  enhancedPerformance.mark = mark;
  enhancedPerformance.measure = measure;
  enhancedPerformance.getEntriesByType = getEntriesByType;
  enhancedPerformance.getEntriesByName = getEntriesByName;
  enhancedPerformance.clearMarks = clearMarks;
  enhancedPerformance.clearMeasures = clearMeasures;

  return enhancedPerformance;
}

export {
  createQuickjsPerformance,
  mark,
  measure,
  getEntriesByType,
  getEntriesByName,
  clearMarks,
  clearMeasures,
};
