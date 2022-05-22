import { ElectionEvent, ElectionEventType } from '../leader/leader'
import { get, set } from 'idb-keyval'

// recheckPeriod is how frequently to re-check the leader (ms).
// this rechecks the expirePeriod on the latest leader claim.
// if we are the leader, this updates the leader claim timestamp.
// note: the leader usually will send a STEP_DOWN message.
const recheckPeriod = 500

// expirePeriod is how long before a leader claim expires.
// note: the leader usually will send a STEP_DOWN message.
const expirePeriod = 2000

// idPrefix is the prefix used for identifiers.
const idPrefix = 'bldr/leader-elect/'

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
  // recheckInterval is the setInterval value for the recheck.
  private recheckInterval: NodeJS.Timer
  // leaderKey is the key used to hold the leader uuid.
  private leaderKey: string
  // workerListKeyPrefix is the prefix for the worker list.
  private workerListKeyPrefix: string
  // workerListKey is the key containing data for this worker.
  private workerListKey: string
  // workerDataKeyPrefix is the prefix for the worker data.
  private workerDataKeyPrefix: string
  // recheckStepUp indicates we should step up if leader is unset.
  private recheckStepUp: boolean
  // currLeader is the current leader as observed by checkLeader.
  private currLeader: string
  // leaderCallback is called when currLeader changes.
  private leaderCallback: LeaderCallback

  constructor(
    electionUuid: string,
    workerUuid: string,
    leaderCallback: LeaderCallback
  ) {
    this.leaderCallback = leaderCallback
    if (!electionUuid) {
      electionUuid = 'bldr-runtime'
    }
    this.electionUuid = electionUuid

    this.workerUuid = workerUuid
    if (!this.workerUuid) {
      this.workerUuid = Math.random().toString(36).substr(2, 9)
    }

    // compute the leader key
    this.leaderKey = idPrefix + electionUuid
    this.workerListKeyPrefix = idPrefix + electionUuid + '/w/'
    this.workerListKey = this.workerListKeyForWorker(workerUuid)
    this.workerDataKeyPrefix = idPrefix + electionUuid + '/d/'


    // eslint-disable-next-line
    console.log('starting leader election', this.electionUuid, this.workerUuid)
    this.electionBroadcast = new BroadcastChannel(idPrefix + electionUuid)
    this.electionBroadcast.onmessage =
      this.onElectionBroadcastMessage.bind(this)

    // check the leader initial state
    this.recheckStepUp = true
    this.currLeader = ''
    this.checkLeader(this.recheckStepUp).then((initLeader) => {
      if (!initLeader && !this.recheckStepUp) {
        leaderCallback('', false)
      }
    })

    // re-check the leader every leaderPeriod ms.
    // jitter: 300ms
    this.recheckInterval = setInterval(
      this.onRecheckInterval.bind(this),
      recheckPeriod + Math.random() * 300
    )
  }

  // close stops the leader election.
  public close() {
    this.stepDown(true)
    if (this.electionBroadcast) {
      this.electionBroadcast.close()
    }
    if (this.recheckInterval) {
      clearInterval(this.recheckInterval)
      this.recheckInterval = null
    }
  }

  // checkLeader checks the leader state and steps up if necessary.
  // returns the current leader.
  // if no leader is set and !stepUp, returns ""
  private async checkLeader(stepUp: boolean): Promise<string> {
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
    const workerUuid = await get<string>(this.leaderKey)
    if (!workerUuid || !workerUuid.length) {
      return undefined
    }
    // Lookup its state.
    return this.getWorkerState(workerUuid)
  }

  // getWorkerState returns the worker state object for a worker.
  // returns undefined if not found
  private async getWorkerState(workerUuid: string): Promise<IWorkerState | undefined> {
    return get<IWorkerState>(workerUuid)
  }

  // setWorkerState sets the worker state for this worker.
  private async setWorkerState(): Promise<void> {
    const state: IWorkerState = {
      id: this.workerUuid,
      ts: new Date(),
    }
    return set(this.workerListKey, state)
  }

  // setLeader sets the current leader key.
  // if id is empty, clears the leader.
  private async setLeader(id: string) {
    let nextValue: string | undefined = undefined
    if (id && id.length) {
      nextValue = id
    }
    return set(this.leaderKey, nextValue)
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
    await this.setWorkerState()
    await this.checkLeader(this.recheckStepUp)
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

  // workerListKeyForWorker returns the worker list key for a given worker
  private workerListKeyForWorker(id: string) {
    return this.workerListKeyPrefix + id
  }

  // workerDataPrefixForWorker returns the worker list key for a given worker
  private workerDataPrefixForWorker(id: string) {
    return this.workerDataKeyPrefix + id
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
