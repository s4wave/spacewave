// See wasm_exec.js from the Go standard library.
declare class Go {
  importObject: WebAssembly.Imports
  env: Record<string, string>
  argv: string[]
  run(inst: WebAssembly.Module): Promise<void>
}

async function deleteDatabase(dbName: string) {
  // Request the deletion of the database
  const request = indexedDB.deleteDatabase(dbName)

  // Create a promise that resolves when the deletion is successful
  await new Promise<void>((resolve, reject) => {
    // Handle success
    request.onsuccess = function () {
      console.log('Database deleted successfully: ' + dbName)
      resolve()
    }
    // Handle error
    request.onerror = function () {
      console.error('Database deletion failed:', request.error)
      reject(request.error)
    }
    // Handle blocked (typically means there are still open connections to the db)
    request.onblocked = function () {
      console.warn('Database deletion is blocked')
    }
  })
}

;(window as any).listDbKeys = function () {
  const dbName = 'hydra/test-db'
  const storeName = 'test-store'

  // Open a connection to the database
  const request = indexedDB.open(dbName)

  request.onsuccess = function (event: any) {
    const db = event!.target!.result!
    const transaction = db.transaction([storeName], 'readonly')
    const store = transaction.objectStore(storeName)

    // Get all keys from the store
    const keysRequest = store.getAllKeys()

    keysRequest.onsuccess = function () {
      const keys = keysRequest.result
      // Decode each key from Uint8Array to string
      keys.forEach((key: ArrayBuffer) => {
        // Decode UTF-8 Uint8Array to string
        const decoder = new TextDecoder('utf-8')
        const keyString = decoder.decode(key)
        console.log(keyString)
      })
    }

    keysRequest.onerror = function (event: any) {
      console.error('Error fetching keys:', event.target.error)
    }

    // Close the database connection when the transaction completes
    transaction.oncomplete = function () {
      db.close()
    }
  }
}

document.addEventListener('DOMContentLoaded', async () => {
  // delete the test db
  await deleteDatabase('hydra/test-kv')
  await deleteDatabase('hydra/test-kvtx')

  const wasmModule = await WebAssembly.compileStreaming(fetch('main.wasm'))
  const runButton = document.getElementById('run-button')!
  runButton.addEventListener('click', async () => {
    const go = new Go()
    const instance = await WebAssembly.instantiate(wasmModule, go.importObject)
    go.run(instance)
  })
})
