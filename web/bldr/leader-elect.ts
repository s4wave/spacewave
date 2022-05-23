import { ElectionEvent, ElectionEventType } from '../leader/leader'
import { IDBKeyRangeWithPrefix } from './idb-prefix'
import { IDBPDatabase, openDB } from 'idb'

// recheckPeriod is how frequently to re-check the leader (ms).
// this rechecks the expirePeriod on the latest leader claim.
// if we are the leader, this updates the leader claim timestamp.
// note: the leader usually will send a STEP_DOWN message.
const recheckPeriod = 500

// recheckJitter is a random amount of delay to add to recheckPeriod.
const recheckJitter = 300

// expirePeriod is how long before a leader claim expires.
// note: the leader usually will send a STEP_DOWN message.
const expirePeriod = 2000

// idPrefix is the prefix used for the database
const idPrefix = 'bldr/leader-elect/'

// leaderStore is the store used for the leader key
const leaderStore = 'leader'

// leaderKey is the key that holds the current leader id
const leaderKey = 'leader'

// workerListStore is the store used for the worker list
const workerListStore = 'worker/list'

// workerDataStore is the store used for the worker data
const workerDataStore = 'worker/data'

// IWorkerState is the object stored in the IndexedDB worker list.
interface IWorkerState {
  // id is the identifier for the worker
  id: string
  // ts is the timestamp, updated periodically.
  // if ts is too long ago, assume the leader has exited.
  ts: Date
}

// LeaderCallback is called when the leader changes.
export type LeaderCallback = (workerId: string, isUs: boolean) => void

// AnnounceCallback is called when a worker announces its presence.
export type AnnounceCallback = (workerId: string) => void

// LeaderElect manages electing a leader from a group of workers.
//
// It accepts two IDs: the election ID and the worker ID.
// Callers should call close() on "onbeforeunload" event.
export class LeaderElect {
  // electionUuid is the unique id of the election.
  private electionUuid: string
  // workerUuid is the unique id of the runtime worker.
  private workerUuid: string
  // electionBroadcast is the election broadcast channel.
  private electionBroadcast: BroadcastChannel
  // db is the indexeddb database for leader-elect
  private db: Promise<IDBPDatabase<unknown>>
  // recheckInterval is the setInterval value for the recheck.
  private recheckInterval?: NodeJS.Timer
  // recheckStepUp indicates we should step up if leader is unset.
  private recheckStepUp?: boolean
  // currLeader is the current leader as observed by checkLeader.
  private currLeader?: string
  // leaderCallback is called when currLeader changes.
  private leaderCallback: LeaderCallback
  // announceCallback is called when a worker announces its presence.
  private announceCallback: AnnounceCallback

  constructor(
    electionUuid: string,
    workerUuid: string,
    leaderCallback: LeaderCallback,
    announceCallback: AnnounceCallback
  ) {
    this.leaderCallback = leaderCallback
    this.announceCallback = announceCallback
    this.electionUuid = electionUuid
    this.workerUuid = workerUuid
    if (!this.workerUuid) {
      this.workerUuid = Math.random().toString(36).substr(2, 9)
    }

    // open the db
    this.db = openDB(idPrefix + electionUuid, 1, {
      upgrade(db) {
        db.createObjectStore(leaderStore)
        db.createObjectStore(workerListStore)
        db.createObjectStore(workerDataStore)
      },
    })

    // init the broadcast channel
    this.electionBroadcast = new BroadcastChannel(idPrefix + electionUuid)
    this.electionBroadcast.onmessage =
      this.onElectionBroadcastMessage.bind(this)

    // eslint-disable-next-line
    console.log('starting leader election', this.electionUuid, this.workerUuid)
    this.start()
  }

  public get isLeader(): boolean {
    return this.workerUuid === this.currLeader
  }

  // getWorkerList returns the current list of active workers
  public async getWorkerList(): Promise<IWorkerState[]> {
    const db = await this.db
    const states: IWorkerState[] = []
    // note, this should be possible, but throws errors curently
    /*
    const tx = db.transaction(workerListStore)
    for await (const cursor of tx.store) {
      const value = cursor.value
      if (typeof value !== 'object') {
        continue
      }
   }
   */
    const keys = await db.getAllKeys(workerListStore)
    for (const key of keys) {
      const entry = await this.getWorkerState(key as string)
      if (!entry || !entry.id || !entry.ts || !this.checkTs(entry.ts)) {
        continue
      }
      states.push(entry)
    }
    return states
  }

  // getWorkerKey returns a key in a store for a worker.
  // if workerUuid is unset or empty, uses local worker uuid
  public async getWorkerKey<T>(
    workerUuid: string | null,
    key: string
  ): Promise<T | undefined> {
    if (!workerUuid) {
      workerUuid = this.workerUuid
    }

    const workerDataPrefix = this.workerDataPrefixForWorker(workerUuid)
    const workerDataKey = workerDataPrefix + key
    const db = await this.db
    return await db.get(workerDataStore, workerDataKey)
  }

  // setWorkerKey sets a key in a store for a worker.
  // if workerUuid is unset or empty, uses local worker uuid
  public async setWorkerKey<T>(
    workerUuid: string | null,
    key: string,
    value: T
  ): Promise<void> {
    if (!workerUuid) {
      workerUuid = this.workerUuid
    }

    const workerDataPrefix = this.workerDataPrefixForWorker(workerUuid)
    const workerDataKey = workerDataPrefix + key
    const db = await this.db
    await db.put(workerDataStore, value, workerDataKey)
  }

  // deleteWorkerKey removes a key in a store for a worker.
  // if workerUuid is unset or empty, uses local worker uuid
  public async deleteWorkerKey(
    workerUuid: string | null,
    key: string
  ): Promise<void> {
    if (!workerUuid) {
      workerUuid = this.workerUuid
    }

    const workerDataPrefix = this.workerDataPrefixForWorker(workerUuid)
    const workerDataKey = workerDataPrefix + key
    const db = await this.db
    await db.delete(workerDataStore, workerDataKey)
  }

  // close stops the leader election.
  public close() {
    this.stepDown(true)
    this.broadcastShutdown()
    if (this.electionBroadcast) {
      this.electionBroadcast.close()
    }
    if (this.recheckInterval) {
      clearInterval(this.recheckInterval)
      delete this.recheckInterval
    }
    this.clearWorkerState(this.workerUuid)
  }

  // start starts all internal routines, called by the constructor
  private async start(): Promise<void> {
    // clear the current state
    await this.clearWorkerState(this.workerUuid)

    // check the leader initial state & write our state
    this.recheckStepUp = true
    this.currLeader = ''
    this.checkLeader(true, this.recheckStepUp)
      .then((initLeader) => {
        if (!initLeader && !this.recheckStepUp && this.leaderCallback) {
          this.leaderCallback('', false)
        }
      })
      .finally(() => {
        // re-check the leader every recheckPeriod + [0, recheckJitter]
        this.recheckInterval = setInterval(
          this.onRecheckInterval.bind(this),
          recheckPeriod + Math.random() * recheckJitter
        )
      })
  }

  // checkLeader checks the leader state and steps up if necessary.
  // returns the current leader.
  // if no leader is set and !stepUp, returns ""
  private async checkLeader(
    announce: boolean,
    stepUp: boolean
  ): Promise<string> {
    // update our state first
    await this.setWorkerListEntry()

    // announce
    if (announce) {
      this.broadcastAnnounce()
    }

    // check current leader
    let currLeader = ''
    let currWorkerState = await this.lookupLeader()
    if (currWorkerState && this.checkTs(currWorkerState.ts)) {
      currLeader = currWorkerState.id
    } else {
      currLeader = ''
    }

    // if a leader is set, return it
    if (!!currLeader || !stepUp) {
      this.updateCurrLeader(currLeader)
      return currLeader
    }

    // step up
    await this.setLeader(this.workerUuid)
    this.broadcastStepUp()
    this.updateCurrLeader(this.workerUuid)
    return this.workerUuid
  }

  // stepDown confirms if we are the leader and if so, steps down.
  // if force=true and this.currLeader == this.workerUuid, skips checks.
  private async stepDown(force: boolean) {
    let currLeader = this.currLeader
    if (!force) {
      currLeader = await this.checkLeader(false, false)
    }
    if (this.workerUuid === currLeader) {
      await this.setLeader(undefined)
      this.broadcastStepDown()
      this.currLeader = ''
    }
  }

  // updateCurrLeader updates the currLeader field and logs any changes.
  private updateCurrLeader(currLeader: string) {
    if (currLeader === this.currLeader) {
      return
    }
    // eslint-disable-next-line
    const localLeader = this.workerUuid == currLeader
    console.log(
      'leader election: leader changed',
      this.electionUuid,
      currLeader,
      localLeader
    )
    this.currLeader = currLeader
    if (this.leaderCallback) {
      this.leaderCallback(currLeader, localLeader)
    }
  }

  // getWorkerState returns the worker state object for a worker.
  // returns undefined if not found
  private async getWorkerState(
    workerUuid: string
  ): Promise<IWorkerState | undefined> {
    const db = await this.db
    return await db.get(workerListStore, workerUuid)
  }

  // lookupLeader looks up the current leader from the IndexedDB.
  // returns undefined if none
  private async lookupLeader(): Promise<IWorkerState | undefined> {
    // Lookup the current leader key.
    const db = await this.db
    const workerUuid = await db.get<string>(leaderStore, leaderKey)
    if (!workerUuid || !workerUuid.length) {
      return undefined
    }
    // Lookup its state.
    return this.getWorkerState(workerUuid)
  }

  // setWorkerListEntry sets the worker list entry for this worker.
  private async setWorkerListEntry(): Promise<void> {
    const state: IWorkerState = {
      id: this.workerUuid,
      ts: new Date(),
    }
    const db = await this.db
    await db.put(workerListStore, state, this.workerUuid)
  }

  // clearWorkerState clears the state for the given worker.
  // also clears any data stored in the workers' store
  private async clearWorkerState(id: string): Promise<void> {
    const db = await this.db
    const workerDataPrefix = this.workerDataPrefixForWorker(id)
    await db.delete(workerDataStore, IDBKeyRangeWithPrefix(workerDataPrefix))
    await db.delete(workerListStore, id)
  }

  // setLeader sets the current leader key.
  // if id is empty, clears the leader.
  private async setLeader(id?: string) {
    let nextValue: string | undefined = undefined
    if (id && id.length) {
      nextValue = id
    }
    const db = await this.db
    return db.put(leaderStore, nextValue, leaderKey)
  }

  // broadcastStepUp broadcasts a new leader.
  private broadcastStepUp() {
    this.broadcastElectionEvent({
      eventType: ElectionEventType.ElectionEventType_LEADER_STEP_UP,
    })
  }

  // broadcastStepDown broadcasts a leader stepping down.
  private broadcastStepDown() {
    this.broadcastElectionEvent({
      eventType: ElectionEventType.ElectionEventType_LEADER_STEP_DOWN,
    })
  }

  // broadcastAnnounce broadcasts the presence of the worker when starting.
  private broadcastAnnounce() {
    this.broadcastElectionEvent({
      eventType: ElectionEventType.ElectionEventType_ANNOUNCE,
    })
  }

  // broadcastShutdown broadcasts a worker shutting down.
  private broadcastShutdown() {
    this.broadcastElectionEvent({
      eventType: ElectionEventType.ElectionEventType_SHUTDOWN,
    })
  }

  // broadcastElectionEvent writes an election event and fills required fields.
  private broadcastElectionEvent(event: Partial<ElectionEvent>) {
    if (this.electionBroadcast) {
      this.electionBroadcast.postMessage(<ElectionEvent>{
        ...event,
        workerId: this.workerUuid,
      })
    }
  }

  // onRecheckInterval is called when the recheck interval is triggered.
  private async onRecheckInterval() {
    return this.checkLeader(false, !!this.recheckStepUp)
  }

  // onElectionBroadcastMessage is called when the BroadcastChannel receives a message.
  private onElectionBroadcastMessage(e: MessageEvent<ElectionEvent>) {
    const data = e.data
    if (
      !data ||
      !data.eventType ||
      !data.workerId ||
      data.workerId === this.workerUuid
    ) {
      return
    }

    // TODO
    // eslint-disable-next-line
    console.log('leader-elect notify rx', this.electionUuid, data)
    switch (e.data.eventType) {
      case ElectionEventType.ElectionEventType_LEADER_STEP_UP:
        this.updateCurrLeader(e.data.workerId)
        break
      case ElectionEventType.ElectionEventType_SHUTDOWN:
      case ElectionEventType.ElectionEventType_LEADER_STEP_DOWN:
        if (this.currLeader === e.data.workerId) {
          this.updateCurrLeader('')
        }
        break
      case ElectionEventType.ElectionEventType_ANNOUNCE:
        this.onWorkerAnnounce(data.workerId)
    }
  }

  // onWorkerAnnounce handles when a worker is announced.
  private onWorkerAnnounce(workerUuid: string) {
    if (this.announceCallback) {
      this.announceCallback(workerUuid)
    }
  }

  // workerDataPrefixForWorker returns the worker list key for a given worker
  private workerDataPrefixForWorker(id: string) {
    return id + '/'
  }

  // checkTs checks if the timestamp is within range.
  // if true: the worker state is still valid.
  private checkTs(ts: Date): boolean {
    if (!ts) {
      return false
    }
    const now = new Date()
    const diffMs = +now - +ts
    return diffMs < expirePeriod
  }
}
