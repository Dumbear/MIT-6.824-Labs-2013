package paxos

import "testing"
import "runtime"
import "strconv"
import "os"
import "time"
import "fmt"
import "math/rand"

func port(tag string, host int) string {
  s := "/var/tmp/px-"
  s += strconv.Itoa(os.Getuid()) + "-"
  s += strconv.Itoa(os.Getpid()) + "-"
  s += tag + "-"
  s += strconv.Itoa(host)
  return s
}

func ndecided(t *testing.T, pxa []*Paxos, seq int) int {
  count := 0
  var v interface{}
  for i := 0; i < len(pxa); i++ {
    if pxa[i] != nil {
      decided, v1 := pxa[i].Get(seq)
      if decided {
        if count > 0 && v != v1 {
          t.Fatalf("decided values do not match; seq=%v i=%v v=%v v1=%v",
            seq, i, v, v1)
        }
        count++
        v = v1
      }
    }
  }
  return count
}

func waitn(t *testing.T, pxa[]*Paxos, seq int, wanted int) {
  for iters := 0; iters < 50; iters++ {
    if ndecided(t, pxa, seq) >= wanted {
      break
    }
    time.Sleep(100 * time.Millisecond)
  }
  nd := ndecided(t, pxa, seq)
  if nd < wanted {
    t.Fatalf("too few decided; seq=%v ndecided=%v wanted=%v", seq, nd, wanted)
  }
}

func waitmajority(t *testing.T, pxa[]*Paxos, seq int) {
  waitn(t, pxa, seq, len(pxa) / 2)
}

func checkmax(t *testing.T, pxa[]*Paxos, seq int, max int) {
  time.Sleep(3 * time.Second)
  nd := ndecided(t, pxa, seq)
  if nd > max {
    t.Fatalf("too many decided; seq=%v ndecided=%v max=%v", seq, nd, max)
  }
}

func cleanup(pxa []*Paxos) {
  for i := 0; i < len(pxa); i++ {
    if pxa[i] != nil {
      pxa[i].Kill()
    }
  }
}

func TestBasic(t *testing.T) {
  runtime.GOMAXPROCS(4)

  const npaxos = 3
  var pxa []*Paxos = make([]*Paxos, npaxos)
  var pxh []string = make([]string, npaxos)
  defer cleanup(pxa)

  for i := 0; i < npaxos; i++ {
    pxh[i] = port("basic", i)
  }
  for i := 0; i < npaxos; i++ {
    pxa[i] = Make(pxh, i, nil)
  }

  fmt.Printf("Single proposer: ")

  pxa[0].Start(0, "hello")
  waitn(t, pxa, 0, npaxos)

  fmt.Printf("OK\n")

  fmt.Printf("Many proposers, same value: ")

  for i := 0; i < npaxos; i++ {
    pxa[i].Start(1, 77)
  }
  waitn(t, pxa, 1, npaxos)

  fmt.Printf("OK\n")

  fmt.Printf("Many proposers, different values: ")

  pxa[0].Start(2, 100)
  pxa[1].Start(2, 101)
  pxa[2].Start(2, 102)
  waitn(t, pxa, 2, npaxos)

  fmt.Printf("OK\n")

  fmt.Printf("Out-of-order instances: ")

  pxa[0].Start(7, 700)
  pxa[0].Start(6, 600)
  pxa[1].Start(5, 500)
  waitn(t, pxa, 7, npaxos)
  pxa[0].Start(4, 400)
  pxa[1].Start(3, 300)
  waitn(t, pxa, 6, npaxos)
  waitn(t, pxa, 5, npaxos)
  waitn(t, pxa, 4, npaxos)
  waitn(t, pxa, 3, npaxos)

  if pxa[0].Max() != 7 {
    t.Fatalf("wrong Max()")
  }

  fmt.Printf("OK\n")
}

func TestGC(t *testing.T) {
  runtime.GOMAXPROCS(4)

  const npaxos = 6
  var pxa []*Paxos = make([]*Paxos, npaxos)
  var pxh []string = make([]string, npaxos)
  defer cleanup(pxa)
  
  for i := 0; i < npaxos; i++ {
    pxh[i] = port("gc", i)
  }
  for i := 0; i < npaxos; i++ {
    pxa[i] = Make(pxh, i, nil)
  }

  fmt.Printf("Garbage collection: ")

  // initial Min() correct?
  for i := 0; i < npaxos; i++ {
    m := pxa[i].Min()
    if m > 0 {
      t.Fatalf("wrong initial Min() %v", m)
    }
  }

  pxa[0].Start(0, "00")
  pxa[1].Start(1, "11")
  pxa[2].Start(2, "22")
  pxa[0].Start(6, "66")
  pxa[1].Start(7, "77")

  waitn(t, pxa, 0, npaxos)

  // Min() correct?
  for i := 0; i < npaxos; i++ {
    m := pxa[i].Min()
    if m != 0 {
      t.Fatalf("wrong Min() %v; expected 0", m)
    }
  }

  waitn(t, pxa, 1, npaxos)

  // Min() correct?
  for i := 0; i < npaxos; i++ {
    if pxa[i].Min() != 0 {
      t.Fatalf("wrong Min()")
    }
  }

  // everyone Done() -> Min() changes?
  for i := 0; i < npaxos; i++ {
    pxa[i].Done(0)
  }
  for i := 1; i < npaxos; i++ {
    pxa[i].Done(1)
  }
  for i := 0; i < npaxos; i++ {
    pxa[i].Start(8 + i, "xx")
  }
  allok := false
  for iters := 0; iters < 12; iters++ {
    allok = true
    for i := 0; i < npaxos; i++ {
      s := pxa[i].Min()
      if s != 1 {
        allok = false
      }
    }
    if allok {
      break
    }
    time.Sleep(1 * time.Second)
  }
  if allok != true {
    t.Fatalf("Min() did not advance after Done()")
  }

  fmt.Printf("OK\n")
}


//
// many agreements (without failures)
//
func TestMany(t *testing.T) {
  runtime.GOMAXPROCS(4)

  fmt.Printf("Many instances: ")

  const npaxos = 4
  var pxa []*Paxos = make([]*Paxos, npaxos)
  var pxh []string = make([]string, npaxos)
  defer cleanup(pxa)

  for i := 0; i < npaxos; i++ {
    pxh[i] = port("many", i)
  }
  for i := 0; i < npaxos; i++ {
    pxa[i] = Make(pxh, i, nil)
    pxa[i].Start(0, 0)
  }

  const ninst = 50
  for seq := 1; seq < ninst; seq++ {
    for i := 0; i < npaxos; i++ {
      pxa[i].Start(seq, (seq * 10) + i)
    }
  }

  for {
    done := true
    for seq := 1; seq < ninst; seq++ {
      if ndecided(t, pxa, seq) < npaxos {
        done = false
      }
    }
    if done {
      break
    }
    time.Sleep(100 * time.Millisecond)
  }

  fmt.Printf("OK\n")
}

//
// basic failure / restart cases
//
func TestBasicCrash(t *testing.T) {
  runtime.GOMAXPROCS(4)

  const npaxos = 5
  var pxa []*Paxos = make([]*Paxos, npaxos)
  var pxh []string = make([]string, npaxos)
  defer cleanup(pxa)

  for i := 0; i < npaxos; i++ {
    pxh[i] = port("basiccrash", i)
  }
  for i := 0; i < npaxos; i++ {
    pxa[i] = Make(pxh, i, nil)
    pxa[i].Start(0, 0)
  }

  fmt.Printf("Minority crash: ")

  pxa[0].Kill()
  pxa[1].Kill()
  pxa[2].Start(1, 101)
  waitmajority(t, pxa, 1)

  fmt.Printf("OK\n")

  fmt.Printf("Majority crash: ")

  pxa[2].Kill()
  pxa[3].Start(2, 102)
  checkmax(t, pxa, 2, 0)

  fmt.Printf("OK\n")

  fmt.Printf("After reboot: ")

  pxa[1] = Make(pxh, 1, nil)
  pxa[2] = Make(pxh, 2, nil)
  pxa[3].Start(3, 103)

  // do agreements now complete?
  // XXX requiring completion is a little dubious, since peers that
  // have lost on-disk Paxos state really should not
  // let themselves restart.
  // on the other hand, in this test no crashed peer had
  // made any prepare_ok promise.
  waitmajority(t, pxa, 2)
  waitmajority(t, pxa, 3)

  fmt.Printf("OK\n")

  fmt.Printf("Rebooted servers see past instances: ")

  ok := false
  for iters := 0; iters < 50; iters++ {
    ok = true
    for seq := 0; seq <= 3; seq++ {
      if ndecided(t, pxa, seq) != 4 {
        ok = false
      }
    }
    if ok {
      break
    }
    time.Sleep(100 * time.Millisecond)
  }
  if ok == false {
    t.Fatalf("restarted peer doesn't see old agreements")
  }
  
  fmt.Printf("OK\n")
}

//
// a peer starts up, with proposal, after others decide.
// then another peer starts, without a proposal.
// 
func TestOld(t *testing.T) {
  runtime.GOMAXPROCS(4)

  fmt.Printf("Minority proposal ignored: ")

  const npaxos = 5
  var pxa []*Paxos = make([]*Paxos, npaxos)
  var pxh []string = make([]string, npaxos)
  defer cleanup(pxa)

  for i := 0; i < npaxos; i++ {
    pxh[i] = port("old", i)
  }

  pxa[1] = Make(pxh, 1, nil)
  pxa[2] = Make(pxh, 2, nil)
  pxa[3] = Make(pxh, 3, nil)
  pxa[1].Start(1, 111)

  waitmajority(t, pxa, 1)

  pxa[0] = Make(pxh, 0, nil)
  pxa[0].Start(1, 222)

  waitn(t, pxa, 1, 4)

  if false {
    pxa[4] = Make(pxh, 4, nil)
    waitn(t, pxa, 1, npaxos)
  }

  fmt.Printf("OK\n")
}

//
// many agreements, with unreliable RPC
//
func TestManyUnreliable(t *testing.T) {
  runtime.GOMAXPROCS(4)

  fmt.Printf("Many instances, unreliable RPC: ")

  const npaxos = 4
  var pxa []*Paxos = make([]*Paxos, npaxos)
  var pxh []string = make([]string, npaxos)
  defer cleanup(pxa)

  for i := 0; i < npaxos; i++ {
    pxh[i] = port("many", i)
  }
  for i := 0; i < npaxos; i++ {
    pxa[i] = Make(pxh, i, nil)
    pxa[i].unreliable = true
    pxa[i].Start(0, 0)
  }

  const ninst = 50
  for seq := 1; seq < ninst; seq++ {
    for i := 0; i < npaxos; i++ {
      pxa[i].Start(seq, (seq * 10) + i)
    }
  }

  for {
    done := true
    for seq := 1; seq < ninst; seq++ {
      if ndecided(t, pxa, seq) < npaxos {
        done = false
      }
    }
    if done {
      break
    }
    time.Sleep(100 * time.Millisecond)
  }
  
  fmt.Printf("OK\n")
}

func pp(tag string, src int, dst int) string {
  s := "/var/tmp/px-" + tag + "-"
  s += strconv.Itoa(os.Getuid()) + "-"
  s += strconv.Itoa(os.Getpid()) + "-"
  s += strconv.Itoa(src) + "-"
  s += strconv.Itoa(dst)
  return s
}

func part(t *testing.T, tag string, npaxos int, p1 []int, p2 []int, p3 []int) {
  for i := 0; i < npaxos; i++ {
    for j := 0; j < npaxos; j++ {
      ij := pp(tag, i, j)
      os.Remove(ij)
    }
  }

  pa := [][]int{p1, p2, p3}
  for pi := 0; pi < len(pa); pi++ {
    p := pa[pi]
    for i := 0; i < len(p); i++ {
      for j := 0; j < len(p); j++ {
        ij := pp(tag, p[i], p[j])
        pj := port(tag, p[j])
        err := os.Link(pj, ij)
        if err != nil {
          t.Fatalf("os.Link(%v, %v): %v\n", pj, ij, err)
        }
      }
    }
  }
}

func TestPartition(t *testing.T) {
  runtime.GOMAXPROCS(4)

  tag := "partition"
  const npaxos = 5
  var pxa []*Paxos = make([]*Paxos, npaxos)
  defer cleanup(pxa)

  for i := 0; i < npaxos; i++ {
    var pxh []string = make([]string, npaxos)
    for j := 0; j < npaxos; j++ {
      if j == i {
        pxh[j] = port(tag, i)
      } else {
        pxh[j] = pp(tag, i, j)
      }
    }
    pxa[i] = Make(pxh, i, nil)
  }
  defer part(t, tag, npaxos, []int{}, []int{}, []int{})

  seq := 0

  fmt.Printf("No decision if partitioned: ")

  pxa[1].Start(seq, 111)
  checkmax(t, pxa, seq, 0)
  
  fmt.Printf("OK\n")

  fmt.Printf("Decision in majority partition: ")

  part(t, tag, npaxos, []int{0}, []int{1,2,3}, []int{4})
  time.Sleep(2 * time.Second)
  waitmajority(t, pxa, seq)

  fmt.Printf("OK\n")

  fmt.Printf("All agree after full heal: ")

  pxa[0].Start(seq, 1000) // poke them
  pxa[4].Start(seq, 1004)
  part(t, tag, npaxos, []int{0,1,2,3,4}, []int{}, []int{})

  waitn(t, pxa, seq, npaxos)

  fmt.Printf("OK\n")

  fmt.Printf("One peer switches partitions: ")

  for iters := 0; iters < 20; iters++ {
    seq++

    part(t, tag, npaxos, []int{0,1,2}, []int{3,4}, []int{})
    pxa[0].Start(seq, seq * 10)
    pxa[3].Start(seq, (seq * 10) + 1)
    waitmajority(t, pxa, seq)
    if ndecided(t, pxa, seq) > 3 {
      t.Fatalf("too many decided")
    }
    
    part(t, tag, npaxos, []int{0,1}, []int{2,3,4}, []int{})
    waitn(t, pxa, seq, npaxos)
  }

  fmt.Printf("OK\n")

  fmt.Printf("One peer switches partitions, unreliable: ")

  for i := 0; i < npaxos; i++ {
    pxa[i].unreliable = true
  }

  for iters := 0; iters < 20; iters++ {
    seq++

    part(t, tag, npaxos, []int{0,1,2}, []int{3,4}, []int{})
    pxa[0].Start(seq, seq * 10)
    pxa[3].Start(seq, (seq * 10) + 1)
    waitmajority(t, pxa, seq)
    if ndecided(t, pxa, seq) > 3 {
      t.Fatalf("too many decided")
    }
    
    part(t, tag, npaxos, []int{0,1}, []int{2,3,4}, []int{})
    waitn(t, pxa, seq, 4)
  }

  fmt.Printf("OK\n")
}

func TestLots(t *testing.T) {
  runtime.GOMAXPROCS(4)

  tag := "lots"
  const npaxos = 5
  var pxa []*Paxos = make([]*Paxos, npaxos)
  defer cleanup(pxa)

  for i := 0; i < npaxos; i++ {
    var pxh []string = make([]string, npaxos)
    for j := 0; j < npaxos; j++ {
      if j == i {
        pxh[j] = port(tag, i)
      } else {
        pxh[j] = pp(tag, i, j)
      }
    }
    pxa[i] = Make(pxh, i, nil)
    pxa[i].unreliable = true
  }
  defer part(t, tag, npaxos, []int{}, []int{}, []int{})

  done := false

  // re-partition periodically
  go func() {
    for done == false {
      var a [npaxos]int
      for i := 0; i < npaxos; i++ {
        a[i] = (rand.Int() % 3)
      }
      pa := make([][]int, 3)
      for i := 0; i < 3; i++ {
        pa[i] = make([]int, 0)
        for j := 0; j < npaxos; j++ {
          if a[j] == i {
            pa[i] = append(pa[i], j)
          }
        }
      }
      part(t, tag, npaxos, pa[0], pa[1], pa[2])
      time.Sleep(time.Duration(rand.Int63() % 200) * time.Millisecond)
    }
  }()

  seq := 0

  // periodically start a new instance
  go func () {
    for done == false {
      n := 0
      for i := 0; i < npaxos; i++ {
        if (rand.Int() % 100) < 30 {
          n++
          pxa[i].Start(seq, rand.Int() % 10)
        }
      }
      if n == 0 {
          pxa[0].Start(seq, rand.Int() % 10)
      }
      seq++
      time.Sleep(time.Duration(rand.Int63() % 300) * time.Millisecond)
    }
  }()

  // periodically check that decisions are consistent
  go func() {
    for done == false {
      for i := 0; i < seq; i++ {
        ndecided(t, pxa, i)
      }
      time.Sleep(time.Duration(rand.Int63() % 300) * time.Millisecond)
    }
  }()

  time.Sleep(20 * time.Second)
  done = true
  time.Sleep(2 * time.Second)


  // repair, then check that all instances decided.
  for i := 0; i < npaxos; i++ {
    pxa[i].unreliable = false
  }
  part(t, tag, npaxos, []int{0,1,2,3,4}, []int{}, []int{})
  time.Sleep(2 * time.Second)
  fmt.Printf("done; started %v instances\n", seq)
  time.Sleep(4 * time.Second)


  for i := 0; i < seq; i++ {
    waitmajority(t, pxa, i)
  }
}
