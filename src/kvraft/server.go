package raftkv

import (
	"labgob"
	"labrpc"
	"log"
	"raft"
	"sync"
	"time"
	"bytes"
	"fmt"
)

const Debug = 0

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}


type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Cmd string
	Key string 
	Value string
	ClientId int64
	Seq int
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.

	database map[string]string
	chMap map[int] chan Op
	lastApplied map[int64] int

	appliedRaftLogIndex int // 3B
}


func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	op := Op{
		Cmd : "Get",
		Key: args.Key,
		ClientId : args.ClientId,
		Seq : args.Seq,
	}

	reply.WrongLeader = kv.waitApplying(op,500*time.Millisecond)

	if reply.WrongLeader == false{
		kv.mu.Lock()
		value,ok := kv.database[args.Key]
		kv.mu.Unlock()
		if ok{
			reply.Value = value
			return
		}
		reply.Err = ErrNoKey
	}

}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	op := Op{
		Key : args.Key,
		Value : args.Value,
		Cmd : args.Op,
		ClientId : args.ClientId,
		Seq : args.Seq,
	}
	reply.WrongLeader = kv.waitApplying(op,500 * time.Millisecond)
}

//
// the tester calls Kill() when a KVServer instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (kv *KVServer) Kill() {
	kv.rf.Kill()
	// Your code here, if desired.
}


func (kv *KVServer) waitApplying(op Op,timeout time.Duration) bool{
	// reutrn common part of GetReply and PutAppendReply
	index,_,isLeader := kv.rf.Start(op)
	if isLeader == false{
		return true
	}

	if kv.shouldTakeSnapshot(){
		kv.takeSnapshot()
	}

	var wrongLeader bool

	kv.mu.Lock()
	if _,ok := kv.chMap[index]; !ok{
		kv.chMap[index] = make(chan Op,1)
	}

	ch := kv.chMap[index]
	kv.mu.Unlock()

	select{
	case c := <- ch:
		
		if c.ClientId != op.ClientId || c.Seq != op.Seq{
			wrongLeader = true
		}else{
			wrongLeader = false
		}

	case <-time.After(timeout):
		kv.mu.Lock()
		if kv.isDuplicateRequest(op.ClientId,op.Seq){
			wrongLeader = false
		}else{
			wrongLeader = true
		}
		kv.mu.Unlock()
	}

	kv.mu.Lock()
	delete(kv.chMap,index)
	kv.mu.Unlock()

	return wrongLeader
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate
	// fmt.Printf(" maxraftstate =  %d\n",maxraftstate)

	// You may need initialization code here.

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	// You may need initialization code here.
	kv.database = make(map[string] string)
	kv.chMap = make(map[int] chan Op)
	kv.lastApplied = make(map[int64] int)

	// 3B recover from snapshot
	snapshot := persister.ReadSnapshot()
	kv.installSnapshot(snapshot)

	go func(){
		for msg := range kv.applyCh{
			if msg.CommandValid == false{
				switch msg.Command.(string){
				case "InstallSnapshot":
					kv.installSnapshot(msg.CommandData)
				}
				continue
			}



			op := msg.Command.(Op)

			kv.mu.Lock()
			if kv.isDuplicateRequest(op.ClientId,op.Seq){
				kv.mu.Unlock()
				continue
			}
			switch op.Cmd{
			case "Put":
				kv.database[op.Key] = op.Value
			case "Append":
				kv.database[op.Key] += op.Value
			//Get() does not need to modify database,skip
			}
			kv.lastApplied[op.ClientId] = op.Seq
			kv.appliedRaftLogIndex = msg.CommandIndex

			if ch,ok := kv.chMap[msg.CommandIndex];ok{
				ch <- op
			}
			kv.mu.Unlock()

		}
	}()

	return kv
}

func (kv *KVServer)isDuplicateRequest(clientId int64,seq int) bool{
	lastSeq , ok := kv.lastApplied[clientId]
	if !ok || seq > lastSeq{
		return false
	}
	return true
}


// detect raft log size , decided whether need snapshot
func (kv *KVServer) shouldTakeSnapshot() bool{
	if kv.maxraftstate == -1 {
		return false
	}
	// fmt.Printf("log size : %d  and max : %d\n",kv.rf.GetRaftStateSize(),kv.maxraftstate)
	if kv.rf.GetRaftStateSize() >= kv.maxraftstate{
		return true
	}
	return false
}

// decided take snapshot and encode self state
func (kv *KVServer) takeSnapshot(){
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)

	kv.mu.Lock()
	e.Encode(kv.database)
	e.Encode(kv.lastApplied)
	appliedRaftLogIndex := kv.appliedRaftLogIndex
	kv.mu.Unlock()

	kv.rf.ReplaceLogWithSnapshot(appliedRaftLogIndex,w.Bytes())
}

// apply snapshot to database and lastApplied
func (kv *KVServer) installSnapshot(snapshot []byte){
	kv.mu.Lock()
	defer kv.mu.Unlock()

	if snapshot != nil{
		r := bytes.NewBuffer(snapshot)
		d := labgob.NewDecoder(r)
		if d.Decode(&kv.database) != nil || d.Decode(&kv.lastApplied) != nil{
			fmt.Printf("kvserver %d fails to recover from snapshot\n",kv.me)
		}
	}
}





