# Vite Hot Module Reload (HMR) Implementation

This document provides a comprehensive overview of all files involved in Vite's Hot Module Reload (HMR) implementation and their specific roles.

## Overview

Vite's HMR system consists of three main components:
1. **Server-side**: Detects file changes and sends updates to clients
2. **Client-side**: Receives updates and applies them to the running application
3. **Shared**: Common utilities used by both server and client

## File Structure

### Core HMR Files

#### Types and Interfaces

**`/types/hmrPayload.d.ts`**
- Defines all HMR payload types and interfaces
- Contains definitions for `HotPayload`, `Update`, `ConnectedPayload`, `UpdatePayload`, `FullReloadPayload`, `CustomPayload`, `ErrorPayload`, `PrunePayload`
- Central type definitions used across the entire HMR system

#### Shared Components

**`/src/shared/hmr.ts`**
- Core HMR client implementation shared between browser and module runner environments
- Exports `HMRClient` class: Manages HMR state, module callbacks, and update processing
- Exports `HMRContext` class: Implements `ViteHotContext` interface providing `import.meta.hot` API
- Handles module acceptance, disposal, pruning, and custom event listeners
- Manages update queue to ensure proper ordering of HMR updates

**`/src/shared/hmrHandler.ts`**
- Simple queue implementation for HMR message processing
- Exports `createHMRHandler()`: Creates a queued handler to ensure HMR updates are processed sequentially
- Prevents race conditions when multiple updates are triggered simultaneously

#### Client-side Implementation

**`/src/client/client.ts`**
- Main HMR client implementation for browser environments
- Establishes WebSocket connection to the Vite dev server
- Handles all HMR payload types: `connected`, `update`, `full-reload`, `prune`, `error`, `custom`
- Manages CSS hot updates by replacing `<link>` tags
- Implements JavaScript module hot updates via dynamic imports
- Provides error overlay integration and fallback connection logic
- Exports `createHotContext()` to create `import.meta.hot` contexts for modules
- Exports `updateStyle()` and `removeStyle()` for CSS HMR

**`/src/client/overlay.ts`**
- Error overlay UI component displayed when HMR encounters errors
- Exports `ErrorOverlay` custom element class
- Provides styled error display with stack traces, file locations, and clickable links
- Supports "open in editor" functionality for error locations
- Automatically dismissible via ESC key or clicking outside

#### Server-side Implementation

**`/src/node/server/hmr.ts`**
- Main server-side HMR orchestration and update logic
- Exports `handleHMRUpdate()`: Main entry point for processing file changes
- Exports `updateModules()`: Determines which modules need updates and sends them to clients
- Implements module dependency graph traversal to find update boundaries
- Handles self-accepting modules, accepted dependencies, and circular import detection
- Manages plugin hooks for custom HMR handling
- Exports utility functions for HMR dependency lexing and URL normalization
- Contains propagation algorithm to determine update scope and full reload requirements

**`/src/node/server/ws.ts`**
- WebSocket server implementation for HMR communication
- Exports `createWebSocketServer()`: Creates WebSocket server with HMR capabilities
- Handles client connections, disconnections, and message routing
- Implements token-based security to prevent unauthorized connections
- Provides client management and broadcasting functionality
- Integrates with the normalized hot channel system

#### Module Runner Implementation

**`/src/module-runner/hmrHandler.ts`**
- HMR handler specifically for Vite's module runner (SSR) environment
- Exports `createHMRHandlerForRunner()`: Creates HMR handler for module runner
- Handles module invalidation and re-evaluation in server-side contexts
- Manages entrypoint detection and full program reloads
- Differs from browser client by handling module evaluation rather than script loading

**`/src/module-runner/hmrLogger.ts`**
- Logging utilities for HMR in module runner environments
- Exports `hmrLogger`: Console-based logger for HMR events
- Exports `silentConsole`: No-op logger for silent operation
- Used by module runner HMR client for debug output

#### Supporting Files

**`/src/node/server/moduleGraph.ts`**
- Module dependency graph implementation
- Tracks `lastHMRTimestamp` and `lastHMRInvalidationReceived` for each module
- Used by HMR system to determine module relationships and update boundaries

**`/src/node/server/transformRequest.ts`**
- Request transformation pipeline that integrates with HMR
- Adds HMR timestamps to module URLs to ensure cache busting
- Handles HMR-related query parameters and module warming

**`/src/node/server/environment.ts`**
- Environment abstraction that includes HMR hot channel management
- Integrates WebSocket server with environment lifecycle

**`/src/node/ssr/runtime/serverModuleRunner.ts`**
- Server-side module runner with HMR support
- Creates HMR-enabled runtime for SSR environments

## HMR Flow

### 1. File Change Detection
- File system watcher detects changes
- `handleHMRUpdate()` in `/src/node/server/hmr.ts` is called

### 2. Update Analysis
- Module graph is consulted to find affected modules
- Plugin hooks are called to allow custom filtering
- Update propagation algorithm determines scope of changes

### 3. Update Delivery
- Updates are sent via WebSocket (`/src/node/server/ws.ts`)
- Payload types include module updates, full reloads, or errors

### 4. Client Processing
- Browser client (`/src/client/client.ts`) or module runner (`/src/module-runner/hmrHandler.ts`) receives updates
- Modules are re-imported/re-evaluated
- HMR acceptance callbacks are executed

### 5. State Management
- `HMRClient` manages module state and callbacks
- `HMRContext` provides the `import.meta.hot` API
- Update queue ensures proper execution order

## Key Features

- **Hot Update**: JavaScript and CSS modules can be updated without full page reload
- **Error Handling**: Compilation errors are displayed in an overlay
- **Circular Import Detection**: Prevents HMR issues with circular dependencies
- **Module Acceptance**: Modules can accept updates for themselves or their dependencies
- **Custom Events**: Support for plugin-specific HMR events
- **State Preservation**: Module state can be preserved across updates via `hot.data`
- **Fallback**: Automatic full reload when HMR cannot be applied
- **Security**: Token-based WebSocket authentication

## Integration Points

- **Plugin System**: Plugins can hook into HMR via `hotUpdate` and `handleHotUpdate` hooks
- **Module Resolution**: Works with Vite's module resolution and transformation pipeline
- **Development Server**: Tightly integrated with Vite's dev server architecture
- **Build Tools**: CSS processing, import analysis, and dependency optimization all support HMR

This implementation provides a robust, efficient, and extensible HMR system that supports both browser and server-side JavaScript execution environments.
    
