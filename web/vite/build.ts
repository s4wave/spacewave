import { loadConfigFromFile, mergeConfig, UserConfig } from 'vite'
import { existsSync } from 'node:fs'
import { resolve } from 'path'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ConfigEnv } from 'vitest/config.js'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Load and merge configuration from a specified path if it exists
async function loadOptionalConfig(
  configEnv: ConfigEnv,
  configPath: string,
): Promise<UserConfig | null> {
  if (!existsSync(configPath)) {
    return null
  }

  const loadedConfig = await loadConfigFromFile(configEnv, configPath)
  return loadedConfig?.config || null
}

// Builds a merged vite config from base config and optional additional configs
export async function buildConfig(
  configEnv: ConfigEnv,
  ...additionalConfigPaths: string[]
): Promise<UserConfig> {
  // Load base config
  const baseConfig = await loadConfigFromFile(
    configEnv,
    resolve(__dirname, './vite-base.config.ts'),
  )
  if (!baseConfig) {
    throw new Error('Failed to load base configuration')
  }

  // Start with an empty config
  let mergedConfig: UserConfig = {}

  // Merge additional configurations if they exist
  for (const configPath of additionalConfigPaths) {
    const additionalConfig = await loadOptionalConfig(configEnv, configPath)
    if (additionalConfig) {
      mergedConfig = mergeConfig(mergedConfig, additionalConfig)
    }
  }

  // Apply base configuration last
  return mergeConfig(mergedConfig, baseConfig.config)
}
