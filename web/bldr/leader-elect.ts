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
const idPrefix = "bldr/leader-elect/"

// ILeaderState is the object stored in IndexedDB.
interface ILeaderState {
  // leader is the current leader ID.
  leader: string
  // ts is the timestamp, updated frequently.
  // if ts is too long ago, the leader has exited.
  ts: Date
}

// LeaderCallback is called when the leader changes.
export type LeaderCallback = (leaderId: string, isUs: boolean) => void

// LeaderElect manages electing a leader WebWorker for Bldr.
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
  // leaderKey is the key used to hold the leader uuid & timestamp.
  private leaderKey: string
  // recheckStepUp indicates we should step up if leader is unset.
  private recheckStepUp: boolean
  // currLeader is the current leader as observed by checkLeader.
  private currLeader: string
  // leaderCallback is called when currLeader changes.
  private leaderCallback: LeaderCallback

  constructor(electionUuid: string, workerUuid: string, leaderCallback: LeaderCallback) {
    this.leaderCallback = leaderCallback
    if (!electionUuid) {
      electionUuid = "bldr-runtime"
    }
    this.electionUuid = electionUuid

    this.workerUuid = workerUuid
    if (!this.workerUuid) {
      this.workerUuid = Math.random().toString(36).substr(2, 9)
    }

    // eslint-disable-next-line
    console.log('starting leader election', this.electionUuid, this.workerUuid)
    this.electionBroadcast = new BroadcastChannel(idPrefix + electionUuid)
    this.electionBroadcast.onmessage = this.onElectionBroadcastMessage.bind(this)

    // compute the leader key
    this.leaderKey = idPrefix + electionUuid

    // check the leader initial state
    this.recheckStepUp = true
    this.currLeader = ""
    this.checkLeader(this.recheckStepUp).then((initLeader) => {
      if (!initLeader && !this.recheckStepUp) {
        leaderCallback("", false)
      }
    })

    // re-check the leader every leaderPeriod ms.
    // jitter: 300ms
    this.recheckInterval = setInterval(
      this.onRecheckInterval.bind(this),
      recheckPeriod + (Math.random() * 300),
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
    let currLeader = ""
    let currLeaderState = await this.getLeader()
    if (currLeaderState && this.checkTs(currLeaderState.ts)) {
      currLeader = currLeaderState.leader
    } else {
      currLeader = ""
    }

    // we are the leader
    if (currLeader == this.workerUuid) {
      // update the timestamp
      await this.setLeader(this.workerUuid)
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
      localLeader,
    )
    this.currLeader = currLeader
    if (this.leaderCallback) {
      this.leaderCallback(currLeader, localLeader)
    }
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
      this.currLeader = ""
    }
  }

  // checkTs checks if the timestamp is within range.
  // if true: the leader claim is still valid.
  private checkTs(ts: Date): boolean {
    if (!ts) {
      return false
    }
    const now = new Date()
    const diffMs = (+now) - (+ts)
    return diffMs < expirePeriod
  }

  // getLeader looks up the current leader from the IndexedDB.
  // returns undefined
  private async getLeader(): Promise<ILeaderState | undefined> {
    // Lookup the current leader key.
    return get<ILeaderState>(this.leaderKey)
  }

  // setLeader sets the current leader key.
  // if id is empty, clears the leader.
  private async setLeader(id: string) {
    let nextState: ILeaderState = undefined
    if (id) {
      nextState = {
        leader: id,
        ts: new Date(),
      }
    }
    return set(this.leaderKey, nextState)
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
  private onRecheckInterval() {
    this.checkLeader(this.recheckStepUp)
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
}
