// The asterisk (*) marks this as a generator function.
function* idGenerator() {
  let id = 1;
  while (true) {
    // The `yield` keyword pauses the function and returns the value on the right.
    // When `.next()` is called again, it resumes from here.
    yield id++;
  }
}

// --- How to use the generator ---

// Calling a generator function doesn't run it. It creates a generator object.
const gen = idGenerator();

// To get values, you call .next() on the generator object.
console.log(gen.next()); // { value: 1, done: false }
console.log(gen.next()); // { value: 2, done: false }
console.log(gen.next().value); // 3 (accessing the value directly)

// We can use it in a loop.
console.log('Generating the next 5 IDs:');
for (let i = 0; i < 5; i++) {
  // Using console.log, but you can swap for Porffor.numberLog
  console.log(gen.next().value); // 4, 5, 6, 7, 8
}
