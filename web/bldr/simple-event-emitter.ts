// SimpleEventEmitter is a simplified version of EventEmitter.
export class SimpleEventEmitter<
  Events extends Record<string, (...args: any[]) => void>,
> {
  private eventHandlers: { [K in keyof Events]?: Events[K][] } = {}

  protected emit<K extends keyof Events>(
    event: K,
    ...args: Parameters<Events[K]>
  ): boolean {
    const handlers = this.eventHandlers[event] || []
    handlers.forEach((handler) => handler(...args))
    return handlers.length > 0
  }

  public on<K extends keyof Events>(event: K, listener: Events[K]): this {
    if (!this.eventHandlers[event]) {
      this.eventHandlers[event] = []
    }
    this.eventHandlers[event]!.push(listener)
    return this
  }

  public once<K extends keyof Events>(event: K, listener: Events[K]): this {
    const onceWrapper = (...args: Parameters<Events[K]>) => {
      listener(...args)
      this.removeListener(event, onceWrapper as Events[K])
    }
    return this.on(event, onceWrapper as Events[K])
  }

  public removeListener<K extends keyof Events>(
    event: K,
    listener: Events[K],
  ): this {
    const handlers = this.eventHandlers[event]
    if (handlers) {
      const index = handlers.indexOf(listener)
      if (index !== -1) {
        handlers.splice(index, 1)
      }
    }
    return this
  }

  public hasListener<K extends keyof Events>(event: K): boolean {
    const handlers = this.eventHandlers[event]
    return !!handlers && handlers.length > 0
  }
}
