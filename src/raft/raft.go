package raft



//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import "sync"
import "labrpc"
import "time"
import "math/rand"
import "fmt"

// import "bytes"
// import "labgob"



// const state
const Follower  = 1
const Candidate  = 2
const Leader  = 3

// const time setup
const heartbeatInterval = time.Duration(120) * time.Millisecond
const electionTimeoutLower = time.Duration(300) * time.Millisecond
const electionTimeoutUpper = time.Duration(400) * time.Millisecond


//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	currentTerm int  //2A
	voteFor int //2A
	state int //2A
	heartbeatTimer *time.Timer //2A
	electionTimer *time.Timer //2A

}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	var term int
	var isleader bool
	// Your code here (2A).
	term = rf.currentTerm
	isleader = rf.state == Leader  
	return term, isleader
}


//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}


//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}




//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).

	Term int //2A
	CandidateId int//2A

}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term int //2A
	VoteGranted bool //2A
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm || (args.Term == rf.currentTerm && rf.voteFor != -1 && rf.voteFor != args.CandidateId) {
		reply.VoteGranted = false
	}else{
		rf.currentTerm = args.Term
		rf.convertTo(Follower)
		reply.VoteGranted = true
		rf.voteFor = args.CandidateId
	}
	return

}




//
// example AppendEntries RPC arguments structure.
// field names must start with capital letters!
//
type AppendEntriesArgs struct {
	// Your data here (2A, 2B).

	Term int //2A
	LeaderId int//2A

}

//
// example AppendEntries RPC reply structure.
// field names must start with capital letters!
//
type AppendEntriesReply struct {
	// Your data here (2A).
	Term int //2A
	Success bool //2A
}

//
// example AppendEntries RPC handler.
//
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm{
		reply.Success = false
		return
	}

	if args.Term > rf.currentTerm{
		rf.currentTerm = args.Term
		rf.convertTo(Follower)

	}

	// reset election timer even log does not match
	// args.LeaderId is the current term's Leader
	rf.electionTimer.Reset(randTimerDuration(electionTimeoutLower, electionTimeoutUpper))
	

	reply.Success = true
	return
}
//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}


// should be called with a lock
func (rf *Raft) convertTo(s int){
	if rf.state == s{
		return
	}
	rf.state = s

	switch s{

	case Follower:
		rf.voteFor = -1
		rf.heartbeatTimer.Stop()
		rf.electionTimer.Reset(randTimerDuration(electionTimeoutLower,electionTimeoutUpper))
	
	case Candidate:
		rf.startElection()

	case Leader:
		// fmt.Printf("server %d become Leader, term %d\n",rf.me,rf.currentTerm)
		rf.electionTimer.Stop()
		rf.broadcastHeartbeat()
		rf.heartbeatTimer.Reset(heartbeatInterval)
	}

}

// one Candidate begin to send voteRequest and get elected
// should be call with a lock
func (rf *Raft) startElection(){

	rf.currentTerm += 1
	rf.electionTimer.Reset(randTimerDuration(electionTimeoutLower,electionTimeoutUpper))
	// fmt.Printf("server %d start election, term %d\n",rf.me,rf.currentTerm)
	args := RequestVoteArgs{
		Term : rf.currentTerm,
		CandidateId : rf.me,
	}
	voteCount := 0
	
	for i := range rf.peers{
		if i== rf.me {
			voteCount+=1
			rf.voteFor = rf.me
			continue
		}

		go func(server int){
			var reply RequestVoteReply
			if rf.sendRequestVote(server,&args,&reply){
				rf.mu.Lock()
				if reply.VoteGranted && rf.state == Candidate {
					voteCount+=1
					// fmt.Printf("%d getting one vote,sum : %d\n",rf.me,voteCount)
					if(voteCount > len(rf.peers)/2){
						rf.convertTo(Leader)
					}
				}else{
					if reply.Term > rf.currentTerm{
						rf.currentTerm = reply.Term
						rf.convertTo(Follower)
					}
				}
				rf.mu.Unlock()
			}
		}(i)
	}

}

func (rf *Raft) broadcastHeartbeat(){

	args := AppendEntriesArgs{
		Term : rf.currentTerm,
		LeaderId: rf.me,
	}
	for i:= range rf.peers{
		if i == rf.me{
			continue
		}

		go func(server int){
			var reply AppendEntriesReply
			if rf.sendAppendEntries(server,&args,&reply){
				rf.mu.Lock()
				if reply.Success && rf.state == Leader{

				}else{
					if reply.Term > rf.currentTerm{
						rf.currentTerm = reply.Term
						rf.convertTo(Follower)
					}
				}
				rf.mu.Unlock()
			}
		}(i)


	}
}
//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).


	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	rf.state = Follower //2A
	rf.currentTerm = 0  //2A
	rf.voteFor = -1		//2A
	rf.heartbeatTimer = time.NewTimer(heartbeatInterval)
	rf.electionTimer = time.NewTimer(randTimerDuration(electionTimeoutLower,electionTimeoutUpper))
	fmt.Printf("creating server %d term %d \n ",rf.me,rf.currentTerm)
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go func(rf *Raft){
		for{
			select{
			case <- rf.electionTimer.C:
				rf.mu.Lock()
				if rf.state == Follower{
					rf.convertTo(Candidate)
				}else{
					rf.startElection()
				}
				rf.mu.Unlock()

			case <- rf.heartbeatTimer.C:
				rf.mu.Lock()
				if rf.state == Leader{
					rf.broadcastHeartbeat()
					rf.heartbeatTimer.Reset(heartbeatInterval)
				}
				rf.mu.Unlock()
			}
		}
	}(rf)

	return rf
}

func randTimerDuration(electionTimeoutLower,electionTimeoutUpper time.Duration) time.Duration{
	num := rand.Int63n(electionTimeoutUpper.Nanoseconds() - electionTimeoutLower.Nanoseconds()) + electionTimeoutLower.Nanoseconds()
	return time.Duration(num) * time.Nanosecond
}
