import { ElectionEvent, ElectionEventType } from '../leader/leader'
import { IDBKeyRangeWithPrefix } from './idb-prefix'
import { IDBPDatabase, openDB } from 'idb'

// recheckPeriod is how frequently to re-check the leader (ms).
// this rechecks the expirePeriod on the latest leader claim.
// if we are the leader, this updates the leader claim timestamp.
// note: the leader usually will send a STEP_DOWN message.
const recheckPeriod = 500

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
export type LeaderCallback = (leaderId: string, isUs: boolean) => void

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

  constructor(
    electionUuid: string,
    workerUuid: string,
    leaderCallback: LeaderCallback
  ) {
    this.leaderCallback = leaderCallback
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

  // close stops the leader election.
  public close() {
    this.stepDown(true)
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
    this.checkLeader(this.recheckStepUp)
      .then((initLeader) => {
        if (!initLeader && !this.recheckStepUp && this.leaderCallback) {
          this.leaderCallback('', false)
        }
      })
      .finally(() => {
        // re-check the leader every leaderPeriod ms.
        // jitter: 300ms
        this.recheckInterval = setInterval(
          this.onRecheckInterval.bind(this),
          recheckPeriod + Math.random() * 300
        )
      })
  }

  // checkLeader checks the leader state and steps up if necessary.
  // returns the current leader.
  // if no leader is set and !stepUp, returns ""
  private async checkLeader(stepUp: boolean): Promise<string> {
    // update our state first
    await this.setWorkerListEntry()

    // check current leader
    let currLeader = ''
    let currWorkerState = await this.getLeader()
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
    this.broadcastStepUp(this.workerUuid)
    this.updateCurrLeader(this.workerUuid)
    return this.workerUuid
  }

  // stepDown confirms if we are the leader and if so, steps down.
  // if force=true and this.currLeader == this.workerUuid, skips checks.
  private async stepDown(force: boolean) {
    let currLeader = this.currLeader
    if (!force) {
      currLeader = await this.checkLeader(false)
    }
    if (this.workerUuid == currLeader) {
      await this.setLeader(undefined)
      this.broadcastStepDown(this.workerUuid)
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

  // getLeader looks up the current leader from the IndexedDB.
  // returns undefined if none
  private async getLeader(): Promise<IWorkerState | undefined> {
    // Lookup the current leader key.
    const db = await this.db
    const workerUuid = await db.get<string>(leaderStore, leaderKey)
    if (!workerUuid || !workerUuid.length) {
      return undefined
    }
    // Lookup its state.
    return this.getWorkerState(workerUuid)
  }

  // getWorkerState returns the worker state object for a worker.
  // returns undefined if not found
  private async getWorkerState(
    workerUuid: string
  ): Promise<IWorkerState | undefined> {
    const db = await this.db
    return await db.get(workerListStore, workerUuid)
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
  private broadcastStepUp(leaderId: string) {
    if (this.electionBroadcast) {
      this.electionBroadcast.postMessage(<ElectionEvent>{
        eventType: ElectionEventType.ElectionEventType_LEADER_ELECTED,
        leaderId: leaderId,
      })
    }
  }

  // broadcastStepDown broadcasts a leader stepping down.
  private broadcastStepDown(leaderId: string) {
    if (this.electionBroadcast) {
      this.electionBroadcast.postMessage(<ElectionEvent>{
        eventType: ElectionEventType.ElectionEventType_LEADER_STEP_DOWN,
        leaderId: leaderId,
      })
    }
  }

  // onRecheckInterval is called when the recheck interval is triggered.
  private async onRecheckInterval() {
    return this.checkLeader(!!this.recheckStepUp)
  }

  // onElectionBroadcastMessage is called when the BroadcastChannel receives a message.
  private onElectionBroadcastMessage(e: MessageEvent<ElectionEvent>) {
    const data = e.data
    if (!data) {
      return
    }

    // TODO
    // eslint-disable-next-line
    console.log('leader-elect notify rx', this.electionUuid, data)
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
