async function runTest() {
  console.log('Starting test...')

  // Clear previous database if it exists
  await new Promise((resolve, reject) => {
    const req = indexedDB.deleteDatabase('test-db')
    req.onsuccess = () => resolve()
    req.onerror = () => reject(req.error)
  })
  console.log('Cleared old db (if any)')

  // Open IndexedDB
  const dbName = 'test-db'
  const storeName = 'test-store'
  const request = indexedDB.open(dbName, 1)

  request.onupgradeneeded = (event) => {
    const db = event.target.result
    db.createObjectStore(storeName)
    console.log('Created object store')
  }

  const db = await new Promise((resolve, reject) => {
    request.onerror = () => reject(request.error)
    request.onsuccess = () => resolve(request.result)
  })
  console.log('Opened database')

  // Setup test data (same as in kvtest)
  const testData = [
    { k: new TextEncoder().encode('a/1'), v: new TextEncoder().encode('val1') },
    { k: new TextEncoder().encode('a/2'), v: new TextEncoder().encode('val2') },
    { k: new TextEncoder().encode('a/3'), v: new TextEncoder().encode('val3') },
    { k: new TextEncoder().encode('b/1'), v: new TextEncoder().encode('val4') },
    { k: new TextEncoder().encode('b/2'), v: new TextEncoder().encode('val5') },
    { k: new TextEncoder().encode('c/1'), v: new TextEncoder().encode('val6') },
    {
      k: new TextEncoder().encode('foo-1'),
      v: new TextEncoder().encode('foo'),
    },
    {
      k: new TextEncoder().encode('test-1'),
      v: new TextEncoder().encode('testing-1'),
    },
    {
      k: new TextEncoder().encode('test-2'),
      v: new TextEncoder().encode('testing-2'),
    },
  ]

  // Insert test data
  const tx = db.transaction(storeName, 'readwrite')
  const store = tx.objectStore(storeName)

  console.log('Inserting test data...')
  for (const { k, v } of testData) {
    await new Promise((resolve, reject) => {
      const req = store.put(v, k)
      req.onsuccess = () => resolve()
      req.onerror = () => reject(req.error)
    })
  }
  await new Promise((resolve) => {
    tx.oncomplete = () => resolve()
  })
  console.log('Test data inserted')

  // Now test the iterator behavior
  const readTx = db.transaction(storeName, 'readonly')
  const readStore = readTx.objectStore(storeName)

  console.log('\nTesting forward iteration:')
  // Test forward iteration first
  let cursorReq = readStore.openKeyCursor(null, 'nextunique')
  let result = await new Promise((resolve, reject) => {
    cursorReq.onsuccess = (event) => {
      const cursor = event.target.result
      if (cursor) {
        const key = new Uint8Array(cursor.key)
        resolve(new TextDecoder().decode(key))
      } else {
        resolve(null)
      }
    }
    cursorReq.onerror = () => reject(cursorReq.error)
  })
  console.log('First key in forward order: ' + result)

  console.log('\nTesting reverse iteration:')
  // This mirrors the Go code:
  // it = tx.Iterate(ctx, nil, true, true)
  // it.Seek(nil)
  cursorReq = readStore.openKeyCursor(null, 'prevunique')
  result = await new Promise((resolve, reject) => {
    cursorReq.onsuccess = (event) => {
      const cursor = event.target.result
      if (cursor) {
        const key = new Uint8Array(cursor.key)
        resolve(new TextDecoder().decode(key))
      } else {
        resolve(null)
      }
    }
    cursorReq.onerror = () => reject(cursorReq.error)
  })
  console.log('First key in reverse order: ' + result)

  await new Promise((resolve) => {
    readTx.oncomplete = () => resolve()
  })

  console.log('\nTest complete')
  db.close()
}

// Set up the button click handler
document.getElementById('runTest').addEventListener('click', () => {
  // Run the test
  runTest().catch((err) => console.log('Error: ' + err))
})
