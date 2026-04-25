// Errors for world types operations

// ErrTypeIDEmpty is returned if the given type ID was empty
export class ErrTypeIDEmpty extends Error {
  constructor() {
    super('type ID empty')
    this.name = 'ErrTypeIDEmpty'
  }
}

// ErrUnknownObjectType indicates the object type was not known
export class ErrUnknownObjectType extends Error {
  constructor() {
    super('unknown object type')
    this.name = 'ErrUnknownObjectType'
  }
}
