package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"strconv"
	"time"
)

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.

	for {
		args := AcTaskArgs{}
		reply := AcTaskReply{}
		// 请求任务
		ok := call("Coordinator.AcquireTask", &args, &reply)
		if !ok {
			log.Fatal("call rpc error! work exist!")
		}
		if reply.Task_t == MapTask {
			// map任务
			filename := reply.Map_file_name
			file, err := os.Open(filename)
			if err != nil {
				log.Fatalf("cannot open %v", filename)
			}
			content, err := ioutil.ReadAll(file)
			if err != nil {
				log.Fatalf("cannot read %v", filename)
			}
			file.Close()
			kva := mapf(filename, string(content))
			tmp_cont := make([][]KeyValue, reply.Num_reducer)
			for i := 0; i < reply.Num_reducer; i++ {
				tmp_cont[i] = []KeyValue{}
			}
			for _, kv := range kva {
				idx := ihash(kv.Key) % reply.Num_reducer
				tmp_cont[idx] = append(tmp_cont[idx], kv)
			}
			for i := 0; i < reply.Num_reducer; i++ {
				out_file_name := fmt.Sprintf("mr-%d-%d", reply.Task_id, i)
				ofile, err := ioutil.TempFile("", out_file_name)
				if err != nil {
					log.Fatal(err)
				}

				// 编码为json
				enc := json.NewEncoder(ofile)
				for _, kv := range tmp_cont[i] {
					err := enc.Encode(&kv)
					if err != nil {
						log.Fatal(err)
					}
				}
				ofile.Close()
				// rename file atomic
				os.Rename(ofile.Name(), out_file_name)
			}
			go func(task_id int) {
				Done_args := DoneTaskArgs{}
				Done_reply := DoneTaskReply{}
				Done_args.Task_t = MapTask
				Done_args.Task_id = task_id
				ok := call("Coordinator.TaskDone", &Done_args, &Done_reply)
				if !ok {
					log.Fatal("call map done error!")
				}
			}(reply.Task_id)

		} else if reply.Task_t == ReduceTask {
			// reduce 任务
			all_data := make(map[string][]string)
			for i := 0; i < reply.Map_task_num; i++ {
				filename := "mr-" + strconv.Itoa(i) + "-" + strconv.Itoa(reply.Task_id)
				file, err := os.Open(filename)
				if err != nil {
					log.Fatalf("cannot open %v", filename)
				}
				dec := json.NewDecoder(file)
				for {
					var kv KeyValue
					if err := dec.Decode(&kv); err != nil {
						break
					}
					all_data[kv.Key] = append(all_data[kv.Key], kv.Value)
				}
				file.Close()
			}

			reduce_out := []KeyValue{}
			for k, v := range all_data {
				tmp_data := KeyValue{k, reducef(k, v)}
				reduce_out = append(reduce_out, tmp_data)
			}

			out_put_file := "mr-out-" + strconv.Itoa(reply.Task_id)
			ofile, err := ioutil.TempFile("", out_put_file)
			if err != nil {
				log.Fatal(err)
			}

			for _, data := range reduce_out {
				fmt.Fprintf(ofile, "%v %v\n", data.Key, data.Value)
				if err != nil {
					log.Fatal(err)
				}
			}
			ofile.Close()
			os.Rename(ofile.Name(), out_put_file)

			go func(task_id int) {
				args := DoneTaskArgs{}
				reply := DoneTaskReply{}
				args.Task_id = task_id
				args.Task_t = ReduceTask
				ok := call("Coordinator.TaskDone", &args, &reply)
				if !ok {
					log.Fatal("reduce call taskdone error!")
				}
			}(reply.Task_id)
		} else if reply.Task_t == WaitTask {
			// 等待
			time.Sleep(1 * time.Second)
		} else {
			fmt.Println("unrecognized task type!")
			os.Exit(1)
		}
	}
	// uncomment to send the Example RPC to the coordinator.
	// CallExample()

}

//
// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

//
// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
