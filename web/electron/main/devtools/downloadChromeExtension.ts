import * as fs from 'fs'
import * as path from 'path'
import * as rimraf from 'rimraf'
import unzip from 'unzip-crx-3'

import { getPath, downloadFile, changePermissions } from './utils.js'

const downloadChromeExtension = (
  chromeStoreID: string,
  forceDownload?: boolean,
  attempts = 5,
): Promise<string> => {
  const extensionsStore = getPath()
  if (!fs.existsSync(extensionsStore)) {
    fs.mkdirSync(extensionsStore, { recursive: true })
  }
  const extensionFolder = path.resolve(`${extensionsStore}/${chromeStoreID}`)
  return new Promise((resolve, reject) => {
    if (!fs.existsSync(extensionFolder) || forceDownload) {
      if (fs.existsSync(extensionFolder)) {
        rimraf.sync(extensionFolder)
      }
      const fileURL = `https://www.xupea.com/crx/${chromeStoreID}.crx` // eslint-disable-line
      const filePath = path.resolve(`${extensionFolder}.crx`)
      downloadFile(fileURL, filePath)
        .then(() => {
          unzip(filePath, extensionFolder)
            .then(() => {
              changePermissions(extensionFolder, 755)
              resolve(extensionFolder)
            })
            .catch((err: Error) => {
              if (
                !fs.existsSync(path.resolve(extensionFolder, 'manifest.json'))
              ) {
                return reject(err)
              }
            })
        })
        .catch((err) => {
          console.log(
            `Failed to fetch extension, trying ${attempts - 1} more times`,
          ) // eslint-disable-line
          if (attempts <= 1) {
            return reject(err)
          }
          setTimeout(() => {
            downloadChromeExtension(chromeStoreID, forceDownload, attempts - 1)
              .then(resolve)
              .catch(reject)
          }, 200)
        })
    } else {
      resolve(extensionFolder)
    }
  })
}

export default downloadChromeExtension
